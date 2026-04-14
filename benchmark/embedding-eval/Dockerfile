FROM ubuntu:24.04

ENV DEBIAN_FRONTEND=noninteractive
ENV PYTHONDONTWRITEBYTECODE=1
ENV PYTHONUNBUFFERED=1

RUN apt-get update && apt-get install -y --no-install-recommends \
    python3 python3-pip python3-venv git ca-certificates \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /workspace

RUN python3 -m venv /workspace/venv
ENV PATH="/workspace/venv/bin:$PATH"

COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

# Clone the repo
RUN git clone --depth 1 https://github.com/ziraloop/ziraloop.git /workspace/repo

COPY index_codebase.py .
COPY server.py .
COPY benchmark.py .

EXPOSE 8080

# Index at runtime (needs API key)
CMD ["sh", "-c", "python3 index_codebase.py /workspace/repo /workspace/vectors.db && python3 server.py"]
