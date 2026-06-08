package domainchecker

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"sync"
	"time"

	"clever-connect/internal/db/pebble"
	"clever-connect/internal/logger"
	"clever-connect/internal/models"

	"github.com/google/uuid"
)

type DomainResult struct {
	ID            string
	DomainName    string
	Status        string
	IPAddresses   string
	HTTPStatus    int
	LatencyMs     int
	TLSStatus     bool
	TLSExpiryDays int
	LastCheckedAt time.Time
}

type ResultListener func(result DomainResult)

type Engine struct {
	mu           sync.RWMutex
	jobQueue     chan string
	workerCount  int
	isChecking   bool
	cancelFunc   context.CancelFunc
	ctx          context.Context
	listeners    map[string]ResultListener
}

var instance *Engine
var once sync.Once

func GetEngine() *Engine {
	once.Do(func() {
		instance = &Engine{
			workerCount: 50, // max concurrent checks
			listeners:   make(map[string]ResultListener),
		}
	})
	return instance
}

func (e *Engine) RegisterListener(id string, listener ResultListener) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.listeners[id] = listener
}

func (e *Engine) UnregisterListener(id string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	delete(e.listeners, id)
}

func (e *Engine) broadcast(result DomainResult) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	for _, l := range e.listeners {
		go l(result) // async broadcast
	}
}

func (e *Engine) CheckBulk(domains []string) {
	e.mu.Lock()
	if e.isChecking {
		// Stop previous run if necessary or just append?
		// For simplicity, let's just create a new context
		if e.cancelFunc != nil {
			e.cancelFunc()
		}
	}
	e.isChecking = true
	e.ctx, e.cancelFunc = context.WithCancel(context.Background())
	e.jobQueue = make(chan string, len(domains))
	for _, d := range domains {
		e.jobQueue <- d
	}
	close(e.jobQueue)
	e.mu.Unlock()

	var wg sync.WaitGroup
	for i := 0; i < e.workerCount; i++ {
		wg.Add(1)
		go e.worker(e.ctx, &wg)
	}

	go func() {
		wg.Wait()
		e.mu.Lock()
		e.isChecking = false
		e.mu.Unlock()
		logger.Info("DomainChecker", "Bulk check completed")
	}()
}

func (e *Engine) CheckSingle(domainName string) {
	go func() {
		// Just run synchronously in this goroutine
		res := e.evaluateDomain(domainName)
		e.saveAndBroadcast(res)
	}()
}

func (e *Engine) worker(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case domain, ok := <-e.jobQueue:
			if !ok {
				return // Queue closed
			}
			res := e.evaluateDomain(domain)
			e.saveAndBroadcast(res)
		}
	}
}

func (e *Engine) evaluateDomain(domain string) DomainResult {
	result := DomainResult{
		DomainName:    domain,
		Status:        "pending",
		LastCheckedAt: time.Now(),
	}
	
	// Step 1: DNS Resolution
	ips, err := net.LookupIP(domain)
	if err != nil {
		result.Status = "nxdomain"
		return result
	}
	ipStrs := ""
	for i, ip := range ips {
		if i > 0 {
			ipStrs += ","
		}
		ipStrs += ip.String()
	}
	result.IPAddresses = ipStrs

	// Step 2: TCP Ping
	start := time.Now()
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(domain, "443"), 3*time.Second)
	if err != nil {
		// Try 80 if 443 fails
		conn, err = net.DialTimeout("tcp", net.JoinHostPort(domain, "80"), 3*time.Second)
		if err != nil {
			result.Status = "timeout"
			return result
		}
	} else {
		defer conn.Close()
	}
	latency := time.Since(start).Milliseconds()
	result.LatencyMs = int(latency)
	result.Status = "online"

	// Step 3: TLS Handshake (if 443)
	if conn != nil && conn.RemoteAddr().(*net.TCPAddr).Port == 443 {
		tlsConfig := &tls.Config{InsecureSkipVerify: true, ServerName: domain}
		tlsConn := tls.Client(conn, tlsConfig)
		err = tlsConn.Handshake()
		if err == nil {
			certs := tlsConn.ConnectionState().PeerCertificates
			if len(certs) > 0 {
				cert := certs[0]
				// Basic validity check
				if time.Now().Before(cert.NotAfter) && time.Now().After(cert.NotBefore) {
					result.TLSStatus = true
				}
				days := int(cert.NotAfter.Sub(time.Now()).Hours() / 24)
				result.TLSExpiryDays = days
			}
		}
	}

	// Step 4: HTTP Head/Get
	client := http.Client{Timeout: 3 * time.Second}
	resp, err := client.Head("https://" + domain)
	if err != nil {
		resp, err = client.Head("http://" + domain)
	}
	if err == nil && resp != nil {
		result.HTTPStatus = resp.StatusCode
	}

	if result.Status == "online" && result.LatencyMs > 500 {
		// still online, UI will handle yellow color
	}

	return result
}

func (e *Engine) saveAndBroadcast(res DomainResult) {
	existing, err := pebble.GetDomainByName(res.DomainName)
	var dbModel *models.Domain

	if err != nil || existing == nil {
		// Create
		dbModel = &models.Domain{
			ID:            uuid.New().String(),
			DomainName:    res.DomainName,
			Status:        res.Status,
			IPAddresses:   res.IPAddresses,
			HTTPStatus:    res.HTTPStatus,
			LatencyMs:     res.LatencyMs,
			TLSStatus:     res.TLSStatus,
			TLSExpiryDays: res.TLSExpiryDays,
			LastCheckedAt: res.LastCheckedAt,
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
		}
	} else {
		// Update
		dbModel = existing
		dbModel.Status = res.Status
		dbModel.IPAddresses = res.IPAddresses
		dbModel.HTTPStatus = res.HTTPStatus
		dbModel.LatencyMs = res.LatencyMs
		dbModel.TLSStatus = res.TLSStatus
		dbModel.TLSExpiryDays = res.TLSExpiryDays
		dbModel.LastCheckedAt = res.LastCheckedAt
		dbModel.UpdatedAt = time.Now()
	}

	pebble.SaveDomain(dbModel)
	
	// Update struct ID for broadcast
	res.ID = dbModel.ID
	
	// Broadcast
	e.broadcast(res)
}
