package benchmarks

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// BenchmarkConfig holds benchmark configuration
type BenchmarkConfig struct {
	GraphPostURL string
	HasuraURL    string
	AdminSecret  string
}

// GraphQLRequest represents a GraphQL request
type GraphQLRequest struct {
	Query     string                 `json:"query"`
	Variables map[string]interface{} `json:"variables,omitempty"`
}

// GraphQLResponse represents a GraphQL response
type GraphQLResponse struct {
	Data   interface{}   `json:"data"`
	Errors []interface{} `json:"errors,omitempty"`
}

// =============================================================================
// Query Benchmarks
// =============================================================================

// BenchmarkSimpleQuery benchmarks a simple select query
func BenchmarkSimpleQuery(b *testing.B) {
	query := GraphQLRequest{
		Query: `query { users(limit: 10) { id name email } }`,
	}

	client := &http.Client{Timeout: 30 * time.Second}
	url := getTestURL()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		body, _ := json.Marshal(query)
		req, _ := http.NewRequest("POST", url+"/v1/graphql", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Admin-Secret", getAdminSecret())

		resp, err := client.Do(req)
		if err != nil {
			b.Fatalf("Request failed: %v", err)
		}
		resp.Body.Close()
	}
}

// BenchmarkComplexQuery benchmarks a query with filters and ordering
func BenchmarkComplexQuery(b *testing.B) {
	query := GraphQLRequest{
		Query: `
			query {
				users(
					where: {
						_and: [
							{created_at: {_gte: "2024-01-01"}},
							{status: {_eq: "active"}}
						]
					},
					order_by: {created_at: desc},
					limit: 50
				) {
					id
					name
					email
					created_at
				}
			}
		`,
	}

	client := &http.Client{Timeout: 30 * time.Second}
	url := getTestURL()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		body, _ := json.Marshal(query)
		req, _ := http.NewRequest("POST", url+"/v1/graphql", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Admin-Secret", getAdminSecret())

		resp, err := client.Do(req)
		if err != nil {
			b.Fatalf("Request failed: %v", err)
		}
		resp.Body.Close()
	}
}

// BenchmarkAggregateQuery benchmarks an aggregate query
func BenchmarkAggregateQuery(b *testing.B) {
	query := GraphQLRequest{
		Query: `
			query {
				users_aggregate(where: {status: {_eq: "active"}}) {
					aggregate {
						count
						avg { age }
						max { created_at }
						min { created_at }
					}
				}
			}
		`,
	}

	client := &http.Client{Timeout: 30 * time.Second}
	url := getTestURL()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		body, _ := json.Marshal(query)
		req, _ := http.NewRequest("POST", url+"/v1/graphql", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Admin-Secret", getAdminSecret())

		resp, err := client.Do(req)
		if err != nil {
			b.Fatalf("Request failed: %v", err)
		}
		resp.Body.Close()
	}
}

// BenchmarkNestedQuery benchmarks a query with relationships
func BenchmarkNestedQuery(b *testing.B) {
	query := GraphQLRequest{
		Query: `
			query {
				users(limit: 10) {
					id
					name
					orders(limit: 5) {
						id
						total
						items {
							id
							product_name
							quantity
						}
					}
				}
			}
		`,
	}

	client := &http.Client{Timeout: 30 * time.Second}
	url := getTestURL()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		body, _ := json.Marshal(query)
		req, _ := http.NewRequest("POST", url+"/v1/graphql", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Admin-Secret", getAdminSecret())

		resp, err := client.Do(req)
		if err != nil {
			b.Fatalf("Request failed: %v", err)
		}
		resp.Body.Close()
	}
}

// =============================================================================
// Mutation Benchmarks
// =============================================================================

// BenchmarkSingleInsert benchmarks inserting a single row
func BenchmarkSingleInsert(b *testing.B) {
	client := &http.Client{Timeout: 30 * time.Second}
	url := getTestURL()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		query := GraphQLRequest{
			Query: `
				mutation($name: String!, $email: String!) {
					insert_benchmark_users_one(object: {name: $name, email: $email}) {
						id
						name
						email
					}
				}
			`,
			Variables: map[string]interface{}{
				"name":  fmt.Sprintf("User %d", i),
				"email": fmt.Sprintf("user%d@example.com", i),
			},
		}

		body, _ := json.Marshal(query)
		req, _ := http.NewRequest("POST", url+"/v1/graphql", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Admin-Secret", getAdminSecret())

		resp, err := client.Do(req)
		if err != nil {
			b.Fatalf("Request failed: %v", err)
		}
		resp.Body.Close()
	}
}

// BenchmarkBatchInsert benchmarks batch insert
func BenchmarkBatchInsert(b *testing.B) {
	client := &http.Client{Timeout: 30 * time.Second}
	url := getTestURL()

	// Create 100 users per batch
	objects := make([]map[string]interface{}, 100)
	for j := 0; j < 100; j++ {
		objects[j] = map[string]interface{}{
			"name":  fmt.Sprintf("BatchUser %d", j),
			"email": fmt.Sprintf("batch%d@example.com", j),
		}
	}

	query := GraphQLRequest{
		Query: `
			mutation($objects: [benchmark_users_insert_input!]!) {
				insert_benchmark_users(objects: $objects) {
					affected_rows
				}
			}
		`,
		Variables: map[string]interface{}{
			"objects": objects,
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		body, _ := json.Marshal(query)
		req, _ := http.NewRequest("POST", url+"/v1/graphql", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Admin-Secret", getAdminSecret())

		resp, err := client.Do(req)
		if err != nil {
			b.Fatalf("Request failed: %v", err)
		}
		resp.Body.Close()
	}
}

// BenchmarkUpdate benchmarks update operations
func BenchmarkUpdate(b *testing.B) {
	client := &http.Client{Timeout: 30 * time.Second}
	url := getTestURL()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		query := GraphQLRequest{
			Query: `
				mutation($id: Int!, $name: String!) {
					update_benchmark_users(
						where: {id: {_eq: $id}},
						_set: {name: $name}
					) {
						affected_rows
					}
				}
			`,
			Variables: map[string]interface{}{
				"id":   i%100 + 1,
				"name": fmt.Sprintf("Updated User %d", i),
			},
		}

		body, _ := json.Marshal(query)
		req, _ := http.NewRequest("POST", url+"/v1/graphql", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Admin-Secret", getAdminSecret())

		resp, err := client.Do(req)
		if err != nil {
			b.Fatalf("Request failed: %v", err)
		}
		resp.Body.Close()
	}
}

// =============================================================================
// Concurrent Benchmarks
// =============================================================================

// BenchmarkConcurrentQueries benchmarks concurrent query execution
func BenchmarkConcurrentQueries(b *testing.B) {
	concurrency := []int{1, 10, 50, 100}

	for _, c := range concurrency {
		b.Run(fmt.Sprintf("concurrency_%d", c), func(b *testing.B) {
			query := GraphQLRequest{
				Query: `query { users(limit: 10) { id name email } }`,
			}

			client := &http.Client{
				Timeout: 30 * time.Second,
				Transport: &http.Transport{
					MaxIdleConns:        c,
					MaxIdleConnsPerHost: c,
				},
			}
			url := getTestURL()

			b.ResetTimer()
			b.SetParallelism(c)
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					body, _ := json.Marshal(query)
					req, _ := http.NewRequest("POST", url+"/v1/graphql", bytes.NewReader(body))
					req.Header.Set("Content-Type", "application/json")
					req.Header.Set("X-Admin-Secret", getAdminSecret())

					resp, err := client.Do(req)
					if err != nil {
						b.Errorf("Request failed: %v", err)
						continue
					}
					resp.Body.Close()
				}
			})
		})
	}
}

// =============================================================================
// Latency Measurement
// =============================================================================

// MeasureLatency measures P50, P95, P99 latencies
func MeasureLatency(b *testing.B, name string, numRequests int) {
	query := GraphQLRequest{
		Query: `query { users(limit: 10) { id name email } }`,
	}

	client := &http.Client{Timeout: 30 * time.Second}
	url := getTestURL()

	latencies := make([]time.Duration, numRequests)

	for i := 0; i < numRequests; i++ {
		start := time.Now()

		body, _ := json.Marshal(query)
		req, _ := http.NewRequest("POST", url+"/v1/graphql", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Admin-Secret", getAdminSecret())

		resp, err := client.Do(req)
		if err != nil {
			continue
		}
		resp.Body.Close()

		latencies[i] = time.Since(start)
	}

	// Sort latencies
	sortDurations(latencies)

	// Calculate percentiles
	p50 := latencies[len(latencies)*50/100]
	p95 := latencies[len(latencies)*95/100]
	p99 := latencies[len(latencies)*99/100]

	fmt.Printf("%s - P50: %v, P95: %v, P99: %v\n", name, p50, p95, p99)
}

// =============================================================================
// Throughput Measurement
// =============================================================================

// MeasureThroughput measures requests per second
func MeasureThroughput(duration time.Duration, concurrency int) {
	query := GraphQLRequest{
		Query: `query { users(limit: 10) { id name email } }`,
	}

	client := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        concurrency,
			MaxIdleConnsPerHost: concurrency,
		},
	}
	url := getTestURL()

	ctx, cancel := context.WithTimeout(context.Background(), duration)
	defer cancel()

	var (
		requestCount int64
		errorCount   int64
		mu           sync.Mutex
		wg           sync.WaitGroup
	)

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				default:
					body, _ := json.Marshal(query)
					req, _ := http.NewRequestWithContext(ctx, "POST", url+"/v1/graphql", bytes.NewReader(body))
					req.Header.Set("Content-Type", "application/json")
					req.Header.Set("X-Admin-Secret", getAdminSecret())

					resp, err := client.Do(req)
					mu.Lock()
					if err != nil {
						errorCount++
					} else {
						requestCount++
						resp.Body.Close()
					}
					mu.Unlock()
				}
			}
		}()
	}

	wg.Wait()

	rps := float64(requestCount) / duration.Seconds()
	fmt.Printf("Throughput: %.2f req/s (errors: %d)\n", rps, errorCount)
}

// =============================================================================
// Database Operation Benchmarks
// =============================================================================

// BenchmarkConnectionPooling benchmarks connection pool efficiency
func BenchmarkConnectionPooling(b *testing.B) {
	query := GraphQLRequest{
		Query: `query { users(limit: 1) { id } }`,
	}

	client := &http.Client{Timeout: 30 * time.Second}
	url := getTestURL()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		body, _ := json.Marshal(query)
		req, _ := http.NewRequest("POST", url+"/v1/graphql", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Admin-Secret", getAdminSecret())

		resp, err := client.Do(req)
		if err != nil {
			b.Fatalf("Request failed: %v", err)
		}
		resp.Body.Close()
	}
}

// BenchmarkLargeResultSet benchmarks fetching large result sets
func BenchmarkLargeResultSet(b *testing.B) {
	limits := []int{100, 500, 1000, 5000}

	for _, limit := range limits {
		b.Run(fmt.Sprintf("limit_%d", limit), func(b *testing.B) {
			query := GraphQLRequest{
				Query: fmt.Sprintf(`query { users(limit: %d) { id name email created_at } }`, limit),
			}

			client := &http.Client{Timeout: 60 * time.Second}
			url := getTestURL()

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				body, _ := json.Marshal(query)
				req, _ := http.NewRequest("POST", url+"/v1/graphql", bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("X-Admin-Secret", getAdminSecret())

				resp, err := client.Do(req)
				if err != nil {
					b.Fatalf("Request failed: %v", err)
				}
				resp.Body.Close()
			}
		})
	}
}

// =============================================================================
// Mock Server for Unit Testing
// =============================================================================

// NewMockServer creates a mock GraphQL server for testing
func NewMockServer() *httptest.Server {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := GraphQLResponse{
			Data: map[string]interface{}{
				"users": []map[string]interface{}{
					{"id": 1, "name": "John", "email": "john@example.com"},
					{"id": 2, "name": "Jane", "email": "jane@example.com"},
				},
			},
		}
		json.NewEncoder(w).Encode(response)
	})

	return httptest.NewServer(handler)
}

// =============================================================================
// Helper Functions
// =============================================================================

func getTestURL() string {
	// Override with environment variable in real tests
	return "http://localhost:8080"
}

func getAdminSecret() string {
	// Override with environment variable in real tests
	return "test-admin-secret"
}

func sortDurations(durations []time.Duration) {
	for i := 0; i < len(durations); i++ {
		for j := i + 1; j < len(durations); j++ {
			if durations[j] < durations[i] {
				durations[i], durations[j] = durations[j], durations[i]
			}
		}
	}
}

// =============================================================================
// Comparison Runner
// =============================================================================

// RunComparison runs benchmark comparison between GraphPost and Hasura
func RunComparison(graphpostURL, hasuraURL, adminSecret string) {
	fmt.Println("=" + "=")
	fmt.Println(" GraphPost vs Hasura Benchmark Comparison")
	fmt.Println("=" + "=")
	fmt.Println()

	tests := []struct {
		name  string
		query string
	}{
		{
			name:  "Simple Query (10 rows)",
			query: `query { users(limit: 10) { id name email } }`,
		},
		{
			name:  "Filtered Query",
			query: `query { users(where: {status: {_eq: "active"}}, limit: 50) { id name } }`,
		},
		{
			name:  "Aggregate Query",
			query: `query { users_aggregate { aggregate { count } } }`,
		},
		{
			name:  "Complex Filter",
			query: `query { users(where: {_and: [{age: {_gte: 18}}, {status: {_eq: "active"}}]}) { id name } }`,
		},
	}

	client := &http.Client{Timeout: 30 * time.Second}

	for _, test := range tests {
		fmt.Printf("\n%s:\n", test.name)
		fmt.Println("-" + "-")

		// Benchmark GraphPost
		graphpostLatency := measureSingleQuery(client, graphpostURL, adminSecret, test.query, 100)

		// Benchmark Hasura
		hasuraLatency := measureSingleQuery(client, hasuraURL, adminSecret, test.query, 100)

		// Calculate difference
		diff := float64(hasuraLatency-graphpostLatency) / float64(hasuraLatency) * 100

		fmt.Printf("  GraphPost:  %v\n", graphpostLatency)
		fmt.Printf("  Hasura:     %v\n", hasuraLatency)
		if diff > 0 {
			fmt.Printf("  GraphPost is %.1f%% faster\n", diff)
		} else {
			fmt.Printf("  Hasura is %.1f%% faster\n", -diff)
		}
	}
}

func measureSingleQuery(client *http.Client, url, adminSecret, query string, iterations int) time.Duration {
	req := GraphQLRequest{Query: query}
	var totalDuration time.Duration

	for i := 0; i < iterations; i++ {
		start := time.Now()

		body, _ := json.Marshal(req)
		httpReq, _ := http.NewRequest("POST", url+"/v1/graphql", bytes.NewReader(body))
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("X-Admin-Secret", adminSecret)

		resp, err := client.Do(httpReq)
		if err != nil {
			continue
		}
		resp.Body.Close()

		totalDuration += time.Since(start)
	}

	return totalDuration / time.Duration(iterations)
}
