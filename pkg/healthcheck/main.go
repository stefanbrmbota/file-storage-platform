// Program healthcheck proverava da li Go servis u istom kontejneru odgovara
// na GET /health (US-33, T-33.3).
//
// Image-i servisa su distroless (bez shell-a i curl-a), pa docker compose ne
// moze da pozove /health obicnom komandom. Zato se ovaj program gradi u build
// stage-u svakog Dockerfile-a i kopira kao /app/healthcheck:
//
//	healthcheck: ["CMD", "/app/healthcheck", "8081"]
//
// Izlazni kod je 0 ako servis vrati 200, inace 1.
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, "upotreba: healthcheck <port>")
		return 1
	}
	port, err := strconv.Atoi(args[0])
	if err != nil || port < 1 || port > 65535 {
		fmt.Fprintln(os.Stderr, "healthcheck: neispravan port")
		return 1
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	url := fmt.Sprintf("http://127.0.0.1:%d/health", port)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		fmt.Fprintln(os.Stderr, "healthcheck:", err)
		return 1
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Fprintln(os.Stderr, "healthcheck: servis ne odgovara")
		return 1
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		fmt.Fprintln(os.Stderr, "healthcheck: status", resp.StatusCode)
		return 1
	}
	return 0
}
