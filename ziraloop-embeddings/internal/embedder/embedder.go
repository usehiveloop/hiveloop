package embedder

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"sync"
	"time"
)

const maxBatchSize = 500

type Embedder struct {
	endpoint string
	model    string
	apiKey   string
	client   *http.Client
}

type BatchTiming struct {
	Batch      int
	Symbols    int
	Tokens     int
	DurationMS int64
}

// embeddingRequest is the OpenAI-compatible request body.
type embeddingRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

type embeddingResponse struct {
	Data  []embeddingData `json:"data"`
	Usage struct {
		TotalTokens int `json:"total_tokens"`
	} `json:"usage"`
}

type embeddingData struct {
	Index     int       `json:"index"`
	Embedding []float32 `json:"embedding"`
}

func New(endpoint, model string) *Embedder {
	return &Embedder{
		endpoint: endpoint,
		model:    model,
		apiKey:   os.Getenv("ZIRALOOP_EMBEDDING_API_KEY"),
		client:   &http.Client{Timeout: 60 * time.Second},
	}
}

// EmbedOne embeds a single text and returns its vector.
func (e *Embedder) EmbedOne(text string) ([]float32, error) {
	embeddings, _, _, err := e.EmbedBatch([]string{text})
	if err != nil {
		return nil, err
	}
	if len(embeddings) == 0 {
		return nil, fmt.Errorf("no embedding returned")
	}
	return embeddings[0], nil
}

// EmbedBatch embeds texts in parallel batches. Returns embeddings in input order,
// total tokens used, and per-batch timing information.
func (e *Embedder) EmbedBatch(texts []string) ([][]float32, int, []BatchTiming, error) {
	if len(texts) == 0 {
		return nil, 0, nil, nil
	}

	// Build batches
	type batch struct {
		index int
		texts []string
	}
	var batches []batch
	for batchStart := 0; batchStart < len(texts); batchStart += maxBatchSize {
		end := batchStart + maxBatchSize
		if end > len(texts) {
			end = len(texts)
		}
		batches = append(batches, batch{index: batchStart, texts: texts[batchStart:end]})
	}

	// Results
	allEmbeddings := make([][]float32, len(texts))
	var totalTokens int
	var timings []BatchTiming
	var mu sync.Mutex
	var wg sync.WaitGroup
	var firstErr error

	for batchIdx, currentBatch := range batches {
		wg.Add(1)
		go func(batchIdx int, currentBatch batch) {
			defer wg.Done()

			start := time.Now()
			embeddings, tokens, err := e.embedOneBatch(currentBatch.texts)
			duration := time.Since(start)

			mu.Lock()
			defer mu.Unlock()

			if err != nil {
				if firstErr == nil {
					firstErr = fmt.Errorf("batch %d: %w", batchIdx+1, err)
				}
				return
			}

			for localIdx, embedding := range embeddings {
				allEmbeddings[currentBatch.index+localIdx] = embedding
			}
			totalTokens += tokens
			timings = append(timings, BatchTiming{
				Batch:      batchIdx + 1,
				Symbols:    len(currentBatch.texts),
				Tokens:     tokens,
				DurationMS: duration.Milliseconds(),
			})
		}(batchIdx, currentBatch)
	}

	wg.Wait()
	if firstErr != nil {
		return nil, 0, nil, firstErr
	}

	sort.Slice(timings, func(i, j int) bool { return timings[i].Batch < timings[j].Batch })
	return allEmbeddings, totalTokens, timings, nil
}

func (e *Embedder) embedOneBatch(texts []string) ([][]float32, int, error) {
	reqBody := embeddingRequest{
		Model: e.model,
		Input: texts,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, 0, fmt.Errorf("marshal request: %w", err)
	}

	url := e.endpoint
	if url[len(url)-1] != '/' {
		url += "/"
	}
	url += "v1/embeddings"

	req, err := http.NewRequest("POST", url, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, 0, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if e.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+e.apiKey)
	}

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, 0, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, 0, fmt.Errorf("API error %d: %s", resp.StatusCode, string(body))
	}

	var embResp embeddingResponse
	if err := json.Unmarshal(body, &embResp); err != nil {
		return nil, 0, fmt.Errorf("unmarshal response: %w", err)
	}

	// Sort by index to maintain input order
	sort.Slice(embResp.Data, func(i, j int) bool {
		return embResp.Data[i].Index < embResp.Data[j].Index
	})

	embeddings := make([][]float32, len(embResp.Data))
	for idx, item := range embResp.Data {
		embeddings[idx] = item.Embedding
	}

	return embeddings, embResp.Usage.TotalTokens, nil
}
