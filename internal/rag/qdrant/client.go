package qdrant

import (
	"time"

	qc "github.com/qdrant/go-client/qdrant"
)

type Config struct {
	Host    string
	Port    int
	UseTLS  bool
	APIKey  string
	Timeout time.Duration
}

type Client struct {
	cfg Config
	c   *qc.Client
}

func New(cfg Config) (*Client, error) {
	if cfg.Port == 0 {
		cfg.Port = 6334
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 120 * time.Second
	}
	c, err := qc.NewClient(&qc.Config{
		Host:   cfg.Host,
		Port:   cfg.Port,
		UseTLS: cfg.UseTLS,
		APIKey: cfg.APIKey,
	})
	if err != nil {
		return nil, err
	}
	return &Client{cfg: cfg, c: c}, nil
}

func (c *Client) Close() error { return c.c.Close() }

func (c *Client) Endpoint() string { return c.cfg.Host }
