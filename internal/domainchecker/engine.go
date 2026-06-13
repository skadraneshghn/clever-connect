package domainchecker

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"strings"
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

type CheckJob struct {
	ID         string
	DomainName string
	Category   string
}

type ResultListener func(result DomainResult)

type Engine struct {
	mu           sync.RWMutex
	jobQueue     chan CheckJob
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
			workerCount: 50, // default concurrent checks
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

func (e *Engine) BroadcastChecking(id, domainName string) {
	e.broadcast(DomainResult{
		ID:         id,
		DomainName: domainName,
		Status:     "checking",
	})
}

func (e *Engine) CheckBulk(domains []models.Domain, threads int, timeoutSec int) {
	e.mu.Lock()
	if e.isChecking {
		if e.cancelFunc != nil {
			e.cancelFunc()
		}
	}
	e.isChecking = true
	e.ctx, e.cancelFunc = context.WithCancel(context.Background())
	
	if threads <= 0 {
		threads = 50
	}
	e.workerCount = threads

	if timeoutSec <= 0 {
		timeoutSec = 3
	}

	e.jobQueue = make(chan CheckJob, len(domains))
	for _, d := range domains {
		e.jobQueue <- CheckJob{
			ID:         d.ID,
			DomainName: d.DomainName,
			Category:   d.Category,
		}
	}
	close(e.jobQueue)
	e.mu.Unlock()

	var wg sync.WaitGroup
	for i := 0; i < e.workerCount; i++ {
		wg.Add(1)
		go e.worker(e.ctx, &wg, timeoutSec)
	}

	go func() {
		wg.Wait()
		e.mu.Lock()
		e.isChecking = false
		e.mu.Unlock()
		logger.Info("DomainChecker", "Bulk check completed")
	}()
}

func (e *Engine) CheckSingle(d *models.Domain) {
	job := CheckJob{
		ID:         d.ID,
		DomainName: d.DomainName,
		Category:   d.Category,
	}
	go func() {
		res := e.evaluateDomain(job, 3)
		e.saveAndBroadcast(res, job.Category)
	}()
}

func (e *Engine) worker(ctx context.Context, wg *sync.WaitGroup, timeoutSec int) {
	defer wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case job, ok := <-e.jobQueue:
			if !ok {
				return // Queue closed
			}
			res := e.evaluateDomain(job, timeoutSec)
			e.saveAndBroadcast(res, job.Category)
		}
	}
}

func (e *Engine) evaluateDomain(job CheckJob, timeoutSec int) DomainResult {
	result := DomainResult{
		ID:            job.ID,
		DomainName:    job.DomainName,
		Status:        "pending",
		LastCheckedAt: time.Now(),
	}
	
	timeout := time.Duration(timeoutSec) * time.Second

	// Step 1: DNS Resolution
	ips, err := net.LookupIP(job.DomainName)
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
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(job.DomainName, "443"), timeout)
	if err != nil {
		// Try 80 if 443 fails
		conn, err = net.DialTimeout("tcp", net.JoinHostPort(job.DomainName, "80"), timeout)
		if err != nil {
			result.Status = "timeout"
			return result
		}
	}
	defer conn.Close()

	latency := time.Since(start).Milliseconds()
	result.LatencyMs = int(latency)
	result.Status = "online"

	// Step 3: TLS Handshake (if port is 443)
	if conn != nil {
		tcpConn, ok := conn.(*net.TCPConn)
		if ok && tcpConn != nil {
			// Set timeout deadline for handshakes
			conn.SetDeadline(time.Now().Add(timeout))
		}
		
		addr := conn.RemoteAddr().String()
		if strings.HasSuffix(addr, ":443") {
			tlsConfig := &tls.Config{InsecureSkipVerify: true, ServerName: job.DomainName}
			tlsConn := tls.Client(conn, tlsConfig)
			err = tlsConn.Handshake()
			if err == nil {
				certs := tlsConn.ConnectionState().PeerCertificates
				if len(certs) > 0 {
					cert := certs[0]
					if time.Now().Before(cert.NotAfter) && time.Now().After(cert.NotBefore) {
						result.TLSStatus = true
					}
					days := int(cert.NotAfter.Sub(time.Now()).Hours() / 24)
					result.TLSExpiryDays = days
				}
			}
		}
	}

	// Step 4: HTTP Head/Get
	client := http.Client{Timeout: timeout}
	resp, err := client.Head("https://" + job.DomainName)
	if err != nil {
		resp, err = client.Head("http://" + job.DomainName)
	}
	if err == nil && resp != nil {
		result.HTTPStatus = resp.StatusCode
		resp.Body.Close()
	}

	return result
}

func (e *Engine) saveAndBroadcast(res DomainResult, category string) {
	existing, err := pebble.GetDomain(res.ID)
	var dbModel *models.Domain

	if err != nil || existing == nil {
		// Fallback lookup
		existing, err = pebble.GetDomainByNameAndCategory(res.DomainName, category)
	}

	if err != nil || existing == nil {
		// Create new if truly not exists
		dbModel = &models.Domain{
			ID:            res.ID,
			DomainName:    res.DomainName,
			Category:      category,
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
		if dbModel.ID == "" {
			dbModel.ID = uuid.New().String()
		}
	} else {
		// Update existing
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
