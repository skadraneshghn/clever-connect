package soroushlib

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"log"
	"math/big"
	"os"
	"strings"
	"sync"
	"time"
)

// ──────────────────────────────────────────────────────────────────────────────
// MTProtoSession — handles encrypted communication over obfuscated transport
// ──────────────────────────────────────────────────────────────────────────────

type rpcResponse struct {
	cid    uint32
	reader *TLReader
	err    error
}

type MTProtoSession struct {
	Transport *ObfuscatedTransport

	AuthKey    []byte
	AuthKeyID  int64
	ServerSalt int64
	SessionID  int64

	seqNo int32
	mu    sync.Mutex

	// Unified multiplexer fields
	rpcWaiters map[int64]chan *rpcResponse
	rpcMu      sync.Mutex
	updateCh   chan UpdateMessage

	readerCtx    context.Context
	readerCancel context.CancelFunc
	readerErr    error
	readerWG     sync.WaitGroup
	readerOnce   sync.Once

	Logger func(msg string, level string)
}

func NewSession(transport *ObfuscatedTransport) *MTProtoSession {
	sid := make([]byte, 8)
	rand.Read(sid)
	return &MTProtoSession{
		Transport:  transport,
		SessionID:  int64(binary.LittleEndian.Uint64(sid)),
		rpcWaiters: make(map[int64]chan *rpcResponse),
		updateCh:   make(chan UpdateMessage, 1000),
	}
}

func (s *MTProtoSession) newMsgID() int64 {
	t := time.Now()
	sec := t.Unix()
	ns := t.UnixNano() - sec*1e9
	return (sec << 32) | (ns & ^int64(3))
}

func (s *MTProtoSession) nextSeq(contentRelated bool) int32 {
	n := s.seqNo * 2
	if contentRelated {
		n += 1
	}
	if contentRelated {
		s.seqNo++
	}
	return n
}

// SendPlain sends an unencrypted MTProto message (for key exchange)
func (s *MTProtoSession) SendPlain(ctx context.Context, body []byte) (int64, error) {
	msgID := s.newMsgID()

	data := make([]byte, 20+len(body))
	binary.LittleEndian.PutUint64(data[8:], uint64(msgID))
	binary.LittleEndian.PutUint32(data[16:], uint32(len(body)))
	copy(data[20:], body)

	return msgID, s.Transport.Send(ctx, data)
}

// RecvPlain receives an unencrypted MTProto response (for key exchange)
func (s *MTProtoSession) RecvPlain(ctx context.Context) ([]byte, error) {
	raw, err := s.Transport.Recv(ctx)
	if err != nil {
		return nil, err
	}
	if len(raw) < 20 {
		return nil, fmt.Errorf("recvPlain: frame too short: %d", len(raw))
	}
	bodyLen := binary.LittleEndian.Uint32(raw[16:20])
	if 20+int(bodyLen) > len(raw) {
		return nil, fmt.Errorf("recvPlain: body extends past frame")
	}
	return raw[20 : 20+bodyLen], nil
}

func (s *MTProtoSession) StartReader(ctx context.Context) {
	s.readerOnce.Do(func() {
		s.readerCtx, s.readerCancel = context.WithCancel(ctx)
		s.readerWG.Add(1)
		go s.readLoop()
	})
}

func (s *MTProtoSession) StopReader() {
	if s.readerCancel != nil {
		s.readerCancel()
	}
	s.readerWG.Wait()
}

func (s *MTProtoSession) readLoop() {
	defer s.readerWG.Done()
	ctx := s.readerCtx

	for {
		select {
		case <-ctx.Done():
			s.readerErr = ctx.Err()
			s.cleanupWaiters(ctx.Err())
			return
		default:
		}

		cid, reader, err := s.Recv(ctx)
		if err != nil {
			if ctx.Err() != nil {
				s.readerErr = ctx.Err()
			} else {
				s.readerErr = err
			}
			s.cleanupWaiters(s.readerErr)
			return
		}

		// Reconstruct raw data for updates if needed
		var rawBytes []byte
		if reader != nil {
			rem := reader.Remaining()
			rawBytes, _ = reader.ReadRaw(rem)
			reader = NewTLReader(rawBytes)
		}

		s.processMessage(cid, reader, rawBytes)
	}
}

func (s *MTProtoSession) processMessage(cid uint32, reader *TLReader, rawBytes []byte) {
	switch cid {
	case IDMsgContainer:
		count, _ := reader.ReadInt32()
		for i := int32(0); i < count; i++ {
			_, _ = reader.ReadInt64()
			reader.ReadInt32() // seq_no
			bodyLen, _ := reader.ReadInt32()
			subBody, err := reader.ReadRaw(int(bodyLen))
			if err != nil {
				continue
			}
			if len(subBody) < 4 {
				continue
			}
			subCID := binary.LittleEndian.Uint32(subBody[:4])
			subReader := NewTLReader(subBody[4:])
			s.processMessage(subCID, subReader, subBody)
		}

	case IDBadServerSalt:
		badMsgID, _ := reader.ReadInt64()
		reader.ReadInt32() // bad_msg_seqno
		reader.ReadInt32() // error_code
		newSalt, _ := reader.ReadInt64()
		s.mu.Lock()
		s.ServerSalt = newSalt
		s.mu.Unlock()
		log.Printf("[MTProto] Reader: Bad server salt, updated to %d", newSalt)

		// Notify waiter for the request that failed
		s.rpcMu.Lock()
		ch, found := s.rpcWaiters[badMsgID]
		if found {
			delete(s.rpcWaiters, badMsgID)
		}
		s.rpcMu.Unlock()

		if found {
			ch <- &rpcResponse{
				err: fmt.Errorf("bad server salt: %d", newSalt),
			}
		}

	case IDNewSession:
		reader.ReadInt64() // first_msg_id
		reader.ReadInt64() // unique_id
		newSalt, _ := reader.ReadInt64()
		s.mu.Lock()
		s.ServerSalt = newSalt
		s.mu.Unlock()
		log.Printf("[MTProto] Reader: New session, salt=%d", newSalt)
		s.sendUpdate(cid, rawBytes)

	case IDRPCResult:
		reqMsgID, _ := reader.ReadInt64()
		innerCID, _ := reader.ReadUint32()

		// Find the waiter for this request
		s.rpcMu.Lock()
		ch, found := s.rpcWaiters[reqMsgID]
		if found {
			delete(s.rpcWaiters, reqMsgID)
		}
		s.rpcMu.Unlock()

		if found {
			ch <- &rpcResponse{
				cid:    innerCID,
				reader: reader,
			}
		} else {
			log.Printf("[MTProto] Reader: Received RPC result for unknown msgID %d", reqMsgID)
			s.sendUpdate(cid, rawBytes)
		}

	default:
		log.Printf("[MTProto] Reader: Unsolicited update received: CID=0x%08X", cid)
		s.sendUpdate(cid, rawBytes)
	}
}

func (s *MTProtoSession) sendUpdate(cid uint32, data []byte) {
	select {
	case s.updateCh <- UpdateMessage{CID: cid, Data: data}:
	default:
		// Drop if buffer full to avoid blocking the main read loop
	}
}

func (s *MTProtoSession) cleanupWaiters(err error) {
	s.rpcMu.Lock()
	defer s.rpcMu.Unlock()
	for msgID, ch := range s.rpcWaiters {
		ch <- &rpcResponse{err: err}
		delete(s.rpcWaiters, msgID)
	}
}

// Send sends an encrypted MTProto message
func (s *MTProtoSession) Send(ctx context.Context, body []byte, contentRelated bool) (int64, error) {
	s.mu.Lock()
	msgID := s.newMsgID()
	seq := s.nextSeq(contentRelated)
	s.mu.Unlock()
	return s.sendWithMsgID(ctx, body, contentRelated, msgID, seq)
}

func (s *MTProtoSession) sendWithMsgID(ctx context.Context, body []byte, contentRelated bool, msgID int64, seq int32) (int64, error) {
	s.mu.Lock()
	salt := s.ServerSalt
	sessionID := s.SessionID
	s.mu.Unlock()

	inner := make([]byte, 32+len(body))
	binary.LittleEndian.PutUint64(inner[0:], uint64(salt))
	binary.LittleEndian.PutUint64(inner[8:], uint64(sessionID))
	binary.LittleEndian.PutUint64(inner[16:], uint64(msgID))
	binary.LittleEndian.PutUint32(inner[24:], uint32(seq))
	binary.LittleEndian.PutUint32(inner[28:], uint32(len(body)))
	copy(inner[32:], body)

	padLen := ((-len(inner) - 12) % 16)
	if padLen < 0 {
		padLen += 16
	}
	padLen += 12
	total := 8 + 16 + len(inner) + padLen
	if total%4 != 0 {
		padLen += 4 - (total % 4)
	}
	padding := make([]byte, padLen)
	rand.Read(padding)
	inner = append(inner, padding...)

	mkBuf := make([]byte, 32+len(inner))
	copy(mkBuf, s.AuthKey[88:120])
	copy(mkBuf[32:], inner)
	msgKeyFull := Sha256Sum(mkBuf)
	msgKey := msgKeyFull[8:24]

	key, iv := GenerateKeyIV(s.AuthKey, msgKey, true)
	enc := AesIGEEncrypt(inner, key, iv)

	packet := make([]byte, 8+16+len(enc))
	binary.LittleEndian.PutUint64(packet[0:], uint64(s.AuthKeyID))
	copy(packet[8:], msgKey)
	copy(packet[24:], enc)

	return msgID, s.Transport.Send(ctx, packet)
}

// SendAndWait sends an encrypted message and waits for the RPC response.
// It automatically handles bad_server_salt by updating the salt and retrying.
// Returns the response constructor ID and TLReader, or an error.
func (s *MTProtoSession) SendAndWait(ctx context.Context, body []byte, contentRelated bool) (uint32, *TLReader, error) {
	s.StartReader(ctx)

	for attempt := 0; attempt < 3; attempt++ {
		if s.readerErr != nil {
			return 0, nil, fmt.Errorf("reader is offline: %w", s.readerErr)
		}

		ch := make(chan *rpcResponse, 1)

		s.mu.Lock()
		msgID := s.newMsgID()
		seq := s.nextSeq(contentRelated)
		s.mu.Unlock()

		s.rpcMu.Lock()
		s.rpcWaiters[msgID] = ch
		s.rpcMu.Unlock()

		_, err := s.sendWithMsgID(ctx, body, contentRelated, msgID, seq)
		if err != nil {
			s.rpcMu.Lock()
			delete(s.rpcWaiters, msgID)
			s.rpcMu.Unlock()
			return 0, nil, fmt.Errorf("send: %w", err)
		}

		select {
		case <-ctx.Done():
			s.rpcMu.Lock()
			delete(s.rpcWaiters, msgID)
			s.rpcMu.Unlock()
			return 0, nil, ctx.Err()
		case resp := <-ch:
			if resp.err != nil {
				if strings.Contains(resp.err.Error(), "bad server salt") {
					log.Printf("[MTProto] SendAndWait: Retrying request %d due to bad salt (attempt %d)...", msgID, attempt+1)
					continue
				}
				return 0, nil, resp.err
			}
			if resp.cid == IDRPCError {
				errorCode, _ := resp.reader.ReadInt32()
				errorMessage, _ := resp.reader.ReadString()
				return resp.cid, resp.reader, fmt.Errorf("RPC error %d: %s", errorCode, errorMessage)
			}
			return resp.cid, resp.reader, nil
		}
	}
	return 0, nil, fmt.Errorf("SendAndWait: failed after 3 retries (bad_server_salt)")
}

// WarmUpSession sends a lightweight RPC request (updates.getState) and handles
// bad_server_salt / new_session_created responses to prime the session salt.
// Call this BEFORE starting ListenForMessages to ensure the salt is correct.
func (s *MTProtoSession) WarmUpSession(ctx context.Context) error {
	w := NewTLWriter()
	w.WriteUint32(0xEDD4882A) // updates.getState
	body := w.GetBytes()

	log.Println("[MTProto] Warming up session (updates.getState)...")
	_, _, err := s.SendAndWait(ctx, body, true)
	if err != nil {
		return fmt.Errorf("warm up: %w", err)
	}
	log.Println("[MTProto] Warm-up successful ✅")
	return nil
}

// Recv receives and decrypts an MTProto message.
// Returns (constructor_id, TLReader, error)
func (s *MTProtoSession) Recv(ctx context.Context) (uint32, *TLReader, error) {
	data, err := s.Transport.Recv(ctx)
	if err != nil {
		return 0, nil, err
	}
	if len(data) < 8 {
		return 0, nil, fmt.Errorf("recv: frame too short: %d bytes", len(data))
	}

	authKeyID := int64(binary.LittleEndian.Uint64(data[0:8]))

	if authKeyID == 0 {
		if len(data) < 20 {
			return 0, nil, fmt.Errorf("recv: unencrypted frame too short")
		}
		bodyLen := int(binary.LittleEndian.Uint32(data[16:20]))
		if 20+bodyLen > len(data) {
			return 0, nil, fmt.Errorf("recv: unencrypted bodyLen=%d exceeds frame len=%d", bodyLen, len(data))
		}
		body := data[20 : 20+bodyLen]
		r := NewTLReader(body)
		cid, err := r.ReadUint32()
		if err != nil {
			return 0, nil, err
		}
		return cid, r, nil
	}

	// Encrypted message
	if len(data) < 24 {
		return 0, nil, fmt.Errorf("recv: encrypted frame too short")
	}
	msgKey := data[8:24]
	enc := data[24:]

	key, iv := GenerateKeyIV(s.AuthKey, msgKey, false)
	inner := AesIGEDecrypt(enc, key, iv)

	if len(inner) < 32 {
		return 0, nil, fmt.Errorf("recv: decrypted inner too short: %d", len(inner))
	}

	bodyLen := int(binary.LittleEndian.Uint32(inner[28:32]))
	if 32+bodyLen > len(inner) {
		log.Printf("[MTProto] WARN: bodyLen exceeds decrypted buffer, clamping (bodyLen=%d, innerLen=%d)", bodyLen, len(inner))
		bodyLen = len(inner) - 32
	}
	body := inner[32 : 32+bodyLen]
	r := NewTLReader(body)
	cid, err := r.ReadUint32()
	if err != nil {
		return 0, nil, err
	}
	return cid, r, nil
}

// ──────────────────────────────────────────────────────────────────────────────
// DH Key Exchange — creates the auth key
// ──────────────────────────────────────────────────────────────────────────────

func (s *MTProtoSession) CreateAuthKey(ctx context.Context) error {
	log.Println("[MTProto] Starting DH key exchange...")

	// Step 1: req_pq_multi
	nonce := make([]byte, 16)
	rand.Read(nonce)

	w := NewTLWriter()
	w.WriteUint32(IDReqPQMulti)
	w.WriteRaw(nonce)
	_, err := s.SendPlain(ctx, w.GetBytes())
	if err != nil {
		return fmt.Errorf("send req_pq_multi: %w", err)
	}

	// Read resPQ
	raw, err := s.RecvPlain(ctx)
	if err != nil {
		return fmt.Errorf("recv resPQ: %w", err)
	}
	r := NewTLReader(raw)
	cid, _ := r.ReadUint32()
	if cid != IDResPQ {
		return fmt.Errorf("expected resPQ (0x%08X), got 0x%08X", IDResPQ, cid)
	}

	_, _ = r.ReadRaw(16) // nonce echo
	srvNonce, _ := r.ReadRaw(16)
	pqBytes, _ := r.ReadBytes()
	pq := new(big.Int).SetBytes(pqBytes)

	// Read fingerprints vector
	_, _ = r.ReadUint32() // vector constructor id
	count, _ := r.ReadInt32()
	fingerprints := make([]uint64, count)
	for i := int32(0); i < count; i++ {
		fp, _ := r.ReadUint64()
		fingerprints[i] = fp
	}

	log.Printf("[MTProto] resPQ received: pq=%s, fingerprints=%v", pq.String(), fingerprints)

	// Step 2: factorize pq
	p, q := factorize(pq.Int64())

	newNonce := make([]byte, 32)
	rand.Read(newNonce)

	pBytes := bigIntToBytes(p)
	qBytes := bigIntToBytes(q)
	pqBytesSer := bigIntToBytes(pq.Int64())

	// Build p_q_inner_data
	inner := NewTLWriter()
	inner.WriteUint32(IDPQInnerData)
	inner.WriteBytes(pqBytesSer)
	inner.WriteBytes(pBytes)
	inner.WriteBytes(qBytes)
	inner.WriteRaw(nonce)
	inner.WriteRaw(srvNonce)
	inner.WriteRaw(newNonce)
	inner.WriteInt32(2) // dc_id = 2
	innerData := inner.GetBytes()

	// Find matching RSA fingerprint
	var fp uint64
	found := false
	for _, f := range fingerprints {
		if _, ok := SoroushRSAKeys[f]; ok {
			fp = f
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("no matching RSA key for fingerprints: %v", fingerprints)
	}

	fp, encrypted, err := RSAEncrypt(innerData, fp)
	if err != nil {
		return fmt.Errorf("rsa encrypt: %w", err)
	}

	// Step 3: req_DH_params
	w = NewTLWriter()
	w.WriteUint32(IDReqDHParams)
	w.WriteRaw(nonce)
	w.WriteRaw(srvNonce)
	w.WriteBytes(pBytes)
	w.WriteBytes(qBytes)
	w.WriteUint64(fp)
	w.WriteBytes(encrypted)
	_, err = s.SendPlain(ctx, w.GetBytes())
	if err != nil {
		return fmt.Errorf("send req_DH_params: %w", err)
	}

	// Read server_DH_params_ok
	raw, err = s.RecvPlain(ctx)
	if err != nil {
		return fmt.Errorf("recv server_DH_params: %w", err)
	}
	r = NewTLReader(raw)
	cid, _ = r.ReadUint32()
	if cid != IDServerDHOK {
		return fmt.Errorf("expected server_DH_params_ok (0x%08X), got 0x%08X", IDServerDHOK, cid)
	}
	r.ReadRaw(16) // nonce
	r.ReadRaw(16) // server_nonce
	encAnswer, _ := r.ReadBytes()

	log.Printf("[MTProto] server_DH_params_ok received (enc_answer_len=%d)", len(encAnswer))

	// Derive tmp_key and tmp_iv
	nn := newNonce
	sn := srvNonce
	shaNNSN := Sha1Sum(append(nn, sn...))
	shaSNNN := Sha1Sum(append(sn, nn...))
	shaNNNN := Sha1Sum(append(nn, nn...))

	tmpKey := append(shaNNSN, shaSNNN[:12]...)
	tmpIV := make([]byte, 0)
	tmpIV = append(tmpIV, shaSNNN[12:]...)
	tmpIV = append(tmpIV, shaNNNN...)
	tmpIV = append(tmpIV, nn[:4]...)

	// Decrypt the answer
	answerFull := AesIGEDecrypt(encAnswer, tmpKey, tmpIV)
	answer := answerFull[20:] // skip SHA1 hash

	// Parse server_DH_inner_data
	ra := NewTLReader(answer)
	got, _ := ra.ReadUint32()
	if got != IDServerDHInnerData {
		return fmt.Errorf("expected server_DH_inner_data (0x%08X), got 0x%08X", IDServerDHInnerData, got)
	}
	ra.ReadRaw(16) // nonce
	ra.ReadRaw(16) // server_nonce
	g, _ := ra.ReadInt32()
	dhPrimeBytes, _ := ra.ReadBytes()
	gABytes, _ := ra.ReadBytes()

	dhPrime := new(big.Int).SetBytes(dhPrimeBytes)
	gA := new(big.Int).SetBytes(gABytes)

	log.Printf("[MTProto] server_DH_inner_data parsed (g=%d)", g)

	// Step 4: Generate client DH
	bBytes := make([]byte, 256)
	rand.Read(bBytes)
	b := new(big.Int).SetBytes(bBytes)

	gBig := big.NewInt(int64(g))
	gB := new(big.Int).Exp(gBig, b, dhPrime)

	authKeyInt := new(big.Int).Exp(gA, b, dhPrime)
	authKeyBytes := make([]byte, 256)
	akb := authKeyInt.Bytes()
	copy(authKeyBytes[256-len(akb):], akb)

	// Calculate server_salt = xor(new_nonce[:8], server_nonce[:8])
	saltBytes := XorBytes(nn[:8], sn[:8])
	s.ServerSalt = int64(binary.LittleEndian.Uint64(saltBytes))

	// Build client_DH_inner_data
	ci := NewTLWriter()
	ci.WriteUint32(IDClientDHInner)
	ci.WriteRaw(nonce)
	ci.WriteRaw(srvNonce)
	ci.WriteInt64(0) // retry_id
	gBBytes := gB.Bytes()
	ci.WriteBytes(gBBytes)
	ciData := ci.GetBytes()

	// Encrypt: sha1(ci_data) + ci_data + padding
	ciEnc := append(Sha1Sum(ciData), ciData...)
	padLen := (-len(ciEnc)) % 16
	if padLen < 0 {
		padLen += 16
	}
	if padLen > 0 {
		ciEnc = append(ciEnc, make([]byte, padLen)...)
	}
	ciEncrypted := AesIGEEncrypt(ciEnc, tmpKey, tmpIV)

	// Send set_client_DH_params
	w = NewTLWriter()
	w.WriteUint32(IDSetClientDH)
	w.WriteRaw(nonce)
	w.WriteRaw(srvNonce)
	w.WriteBytes(ciEncrypted)
	_, err = s.SendPlain(ctx, w.GetBytes())
	if err != nil {
		return fmt.Errorf("send set_client_DH: %w", err)
	}

	// Read dh_gen_ok
	raw, err = s.RecvPlain(ctx)
	if err != nil {
		return fmt.Errorf("recv dh_gen_ok: %w", err)
	}
	r = NewTLReader(raw)
	cid, _ = r.ReadUint32()
	if cid != IDDHGenOK {
		return fmt.Errorf("DH failed: got 0x%08X instead of dh_gen_ok", cid)
	}

	s.AuthKey = authKeyBytes
	akHash := Sha1Sum(authKeyBytes)
	s.AuthKeyID = int64(binary.LittleEndian.Uint64(akHash[12:20]))

	log.Printf("[MTProto] Auth key generated successfully! auth_key_id=%d", s.AuthKeyID)

	return nil
}

// ──────────────────────────────────────────────────────────────────────────────
// Helpers
// ──────────────────────────────────────────────────────────────────────────────

func factorize(pq int64) (int64, int64) {
	if pq%2 == 0 {
		return 2, pq / 2
	}

	rng := make([]byte, 8)
	rand.Read(rng)
	x := int64(binary.LittleEndian.Uint64(rng)%(uint64(pq)-2)) + 2

	rand.Read(rng)
	c := int64(binary.LittleEndian.Uint64(rng)%(uint64(pq)-1)) + 1

	y := x
	d := int64(1)

	for d == 1 {
		x = (mulmod(x, x, pq) + c) % pq
		y = (mulmod(y, y, pq) + c) % pq
		y = (mulmod(y, y, pq) + c) % pq
		diff := x - y
		if diff < 0 {
			diff = -diff
		}
		d = gcd(diff, pq)
	}

	if d == pq {
		for i := int64(3); i*i <= pq; i += 2 {
			if pq%i == 0 {
				return i, pq / i
			}
		}
		fmt.Fprintf(os.Stderr, "factorize: failed for pq=%d\n", pq)
		return 1, pq
	}

	p, q := d, pq/d
	if p > q {
		p, q = q, p
	}
	return p, q
}

func gcd(a, b int64) int64 {
	for b != 0 {
		a, b = b, a%b
	}
	if a < 0 {
		return -a
	}
	return a
}

func mulmod(a, b, m int64) int64 {
	aBig := big.NewInt(a)
	bBig := big.NewInt(b)
	mBig := big.NewInt(m)
	return new(big.Int).Mod(new(big.Int).Mul(aBig, bBig), mBig).Int64()
}

func bigIntToBytes(n int64) []byte {
	bn := big.NewInt(n)
	b := bn.Bytes()
	return b
}

func (s *MTProtoSession) Log(msg string, level string) {
	if s != nil && s.Logger != nil {
		s.Logger(msg, level)
	} else {
		log.Printf("[%s] %s", strings.ToUpper(level), msg)
	}
}
