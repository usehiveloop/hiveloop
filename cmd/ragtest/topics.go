package main

var topics = []struct {
	name      string
	templates []string
}{
	{"javascript", []string{
		"JavaScript closures capture variables by reference. The inner function retains access to the enclosing scope after the outer function returns.",
		"Promises in JavaScript represent eventual completion of asynchronous operations. Use async/await to write linear-looking async code.",
		"The event loop processes tasks from the macrotask and microtask queues. Microtasks (promises) run before the next macrotask.",
		"V8's hidden classes optimize property access. Adding properties in the same order keeps objects on the same hidden class.",
		"WeakMap entries are garbage-collected when their keys lose all other references. Useful for attaching metadata without leaks.",
	}},
	{"python", []string{
		"Python's GIL prevents true thread parallelism for CPU-bound work. Use multiprocessing or asyncio for concurrency.",
		"List comprehensions in Python are generally faster than equivalent for loops because they avoid attribute lookups for append.",
		"Generators use yield to lazily produce values. They preserve state between calls without allocating the full sequence.",
		"Python's dict is implemented as an open-addressing hash table with random-probe sequences and quadratic probing.",
		"Decorators wrap a function with extra behavior. functools.wraps preserves the original function's metadata for introspection.",
	}},
	{"rust", []string{
		"Rust's ownership model prevents data races at compile time. Each value has a single owner and references are tracked by lifetimes.",
		"Borrow checking enforces that mutable references are exclusive. You can have many shared borrows or one mutable borrow, never both.",
		"The Send and Sync traits mark types safe to transfer or share across threads. Most std types implement them automatically.",
		"Rust's enum types model algebraic data. Pattern matching with match guarantees exhaustive handling of every variant.",
		"Cargo workspaces let you share dependency resolution across multiple crates in a monorepo, speeding up builds.",
	}},
	{"go", []string{
		"Goroutines are cheap green threads multiplexed onto OS threads by the Go runtime. Channels coordinate their work.",
		"Go's interface satisfaction is structural. Any type implementing the required method set conforms, no explicit declaration needed.",
		"defer statements run in LIFO order when the function returns. They're commonly used for cleanup like closing files.",
		"Go's garbage collector is concurrent and tri-color mark-sweep. Pause times stay sub-millisecond on most workloads.",
		"Context.WithCancel propagates cancellation through call chains. Deadline-bounded operations should always check ctx.Done().",
	}},
	{"kubernetes", []string{
		"Kubernetes Pods are the smallest schedulable unit. Each Pod gets its own IP and a shared network namespace for its containers.",
		"Deployments manage ReplicaSets that manage Pods. Rolling updates replace Pods incrementally with the new image.",
		"Services provide stable virtual IPs and DNS names for Pod selectors. They abstract over Pod IP churn during scheduling.",
		"ConfigMaps hold non-secret config; Secrets hold sensitive data. Both can mount as files or env vars in containers.",
		"HorizontalPodAutoscaler scales replicas based on CPU, memory, or custom metrics from the metrics-server or Prometheus.",
	}},
	{"postgres", []string{
		"Postgres MVCC keeps multiple row versions to serve concurrent reads without locking. VACUUM reclaims dead tuples afterward.",
		"B-tree indexes support equality and range scans. Hash indexes only support equality and are rarely faster than B-trees.",
		"Postgres uses cost-based query planning. ANALYZE updates statistics; bad row estimates lead to bad plans.",
		"Logical replication streams row-level changes via the WAL. Physical replication ships raw WAL records to standbys.",
		"Common Table Expressions named WITH let you structure complex queries. Postgres 12+ inlines them by default.",
	}},
	{"redis", []string{
		"Redis is single-threaded for commands but uses I/O multiplexing. Throughput scales horizontally via clustering.",
		"Sorted sets order members by score. Use ZADD/ZRANGEBYSCORE for leaderboards and time-bucketed queues.",
		"Redis persistence comes in two flavors: RDB snapshots and AOF append-only files. AOF gives finer-grained durability.",
		"Lua scripts run atomically inside Redis. They serialize complex multi-key transactions without round-trips.",
		"Streams (XADD/XREAD) are append-only logs with consumer groups. Useful for event sourcing and queues with ack semantics.",
	}},
	{"qdrant", []string{
		"Qdrant stores high-dimensional vectors and supports filterable HNSW search. Payload indices speed up filtered queries.",
		"HNSW parameters m and ef_construct trade build time and recall. Higher m gives denser graphs and better recall.",
		"Qdrant's payload supports keywords, integers, geo, full-text, and bool. Indices on filtered fields are essential at scale.",
		"Multi-tenancy in Qdrant uses payload-based partitioning with the is_tenant flag on the org_id field for isolation.",
		"Qdrant's set_payload API mutates point metadata without re-embedding. ACL updates leverage this to avoid index rewrites.",
	}},
	{"machinelearning", []string{
		"Cross-encoder rerankers score query-document pairs jointly. They beat dual-encoder cosine for top-k precision at high latency cost.",
		"Contrastive learning trains embeddings by pulling positives together and pushing negatives apart. InfoNCE is the canonical loss.",
		"Mixture-of-experts routes tokens to specialized subnetworks. Inference cost stays low because only a few experts activate.",
		"Quantization to int8 or int4 cuts memory and bandwidth. Calibration on representative data preserves accuracy.",
		"Retrieval-augmented generation pairs a vector index with an LLM. The retrieved chunks ground the LLM's response in source docs.",
	}},
	{"observability", []string{
		"OpenTelemetry standardizes traces, metrics, and logs. The SDK emits to a collector that exports to your backend of choice.",
		"Distributed tracing tags each request with a trace ID and per-hop span IDs. Spans nest to show causality across services.",
		"Prometheus pulls metrics from /metrics endpoints. Grafana queries Prometheus to render dashboards and alert on thresholds.",
		"Structured logs (JSON) survive parsing better than free-text. Log aggregation pipelines like Loki rely on label cardinality budgets.",
		"Service Level Objectives define the reliability promise. Burn-rate alerts fire when error budget consumption is too fast.",
	}},
}
