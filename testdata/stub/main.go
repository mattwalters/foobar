package main

import (
	"fmt"
	"math/rand"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: stub <process_type>")
		os.Exit(1)
	}

	processType := os.Args[1]
	rand.Seed(time.Now().UnixNano())

	// Simulated startup delay
	fmt.Printf("[%s] Booting up...\n", processType)
	time.Sleep(time.Duration(rand.Intn(1500)+1000) * time.Millisecond)

	// Setup signal handling for simulated graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		fmt.Printf("\n[%s] Received termination signal. Initiating graceful shutdown...\n", processType)
		time.Sleep(time.Duration(rand.Intn(1500)+1000) * time.Millisecond)
		fmt.Printf("[%s] Graceful shutdown complete.\n", processType)
		os.Exit(0)
	}()

	switch processType {
	case "web":
		simulateWeb()
	case "api":
		simulateAPI()
	case "worker":
		simulateWorker()
	case "db":
		simulateDB()
	default:
		fmt.Printf("Unknown process type: %s\n", processType)
		os.Exit(1)
	}
}

func simulateWeb() {
	fmt.Println("ready - started server on 0.0.0.0:3000, url: http://localhost:3000")
	fmt.Println("info  - loaded env from .env.local")
	fmt.Println("event - compiled client and server successfully in 1250 ms (154 modules)")

	routes := []string{"/", "/dashboard", "/settings", "/api/auth/session"}

	for {
		time.Sleep(time.Duration(rand.Intn(3000)+500) * time.Millisecond)
		route := routes[rand.Intn(len(routes))]
		status := 200
		if rand.Float32() > 0.95 {
			status = 500
			fmt.Fprintf(os.Stderr, "error - Failed to render page %s: Internal Server Error\n", route)
		} else if rand.Float32() > 0.8 {
			status = 304
		}

		ms := rand.Intn(150) + 10
		fmt.Printf("Wait  - compiling /page (client and server)... \n")
		time.Sleep(50 * time.Millisecond)
		fmt.Printf("event - compiled client and server successfully in %d ms (159 modules)\n", ms)
		fmt.Printf("GET %s %d in %dms\n", route, status, ms)
	}
}

func simulateAPI() {
	fmt.Println("[info] Starting Go API server...")
	fmt.Println("[info] Connecting to database...")
	time.Sleep(1 * time.Second)
	fmt.Println("[info] Database connected successfully.")
	fmt.Println("[info] Listening on :8080")

	endpoints := []string{"/v1/users", "/v1/posts", "/v1/health"}

	for {
		time.Sleep(time.Duration(rand.Intn(2000)+100) * time.Millisecond)
		endpoint := endpoints[rand.Intn(len(endpoints))]

		if rand.Float32() > 0.9 {
			fmt.Fprintf(os.Stderr, "[error] Failed to fetch data for %s: connection timeout\n", endpoint)
		} else {
			duration := rand.Intn(50) + 2
			fmt.Printf("[info] HTTP %s handled in %dms\n", endpoint, duration)
		}
	}
}

func simulateWorker() {
	fmt.Println("Worker node starting up...")
	fmt.Println("Connecting to Redis queue...")
	time.Sleep(500 * time.Millisecond)
	fmt.Println("Worker ready to process jobs.")

	jobs := []string{"SendWelcomeEmail", "ProcessImageResize", "CalculateAnalytics"}

	for {
		time.Sleep(time.Duration(rand.Intn(5000)+1000) * time.Millisecond)
		job := jobs[rand.Intn(len(jobs))]
		fmt.Printf("[JOB] Received job: %s\n", job)

		processingTime := rand.Intn(2000) + 500
		time.Sleep(time.Duration(processingTime) * time.Millisecond)

		if rand.Float32() > 0.85 {
			fmt.Fprintf(os.Stderr, "[ERROR] Job %s failed after %dms. Retrying in 5s...\n", job, processingTime)
		} else {
			fmt.Printf("[SUCCESS] Job %s completed in %dms\n", job, processingTime)
		}
	}
}

func simulateDB() {
	fmt.Println("PostgreSQL init process complete; ready for start up.")
	fmt.Println("LOG:  database system is ready to accept connections")

	for {
		time.Sleep(time.Duration(rand.Intn(8000)+2000) * time.Millisecond)

		if rand.Float32() > 0.7 {
			fmt.Println("LOG:  checkpoint starting: time")
			time.Sleep(200 * time.Millisecond)
			fmt.Println("LOG:  checkpoint complete: wrote 12 buffers (0.1%); 0 WAL file(s) added, 0 removed, 0 recycled")
		} else {
			fmt.Println("LOG:  authorized: user=appuser database=appdb")
		}
	}
}
