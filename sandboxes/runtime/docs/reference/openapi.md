# OpenAPI

Bridge provides an OpenAPI specification.

---

## Accessing the Spec

### In Repository

```
openapi.json
```

### At Runtime

```bash
curl http://localhost:8080/openapi.json
```

---

## Generating Client Code

### TypeScript

```bash
npx openapi-typescript openapi.json --output bridge-api.ts
```

### Python

```bash
openapi-generator-cli generate \
  -i openapi.json \
  -g python \
  -o bridge-client
```

### Go

```bash
openapi-generator-cli generate \
  -i openapi.json \
  -g go \
  -o bridge-client
```

---

## Regenerating

After API changes:

```bash
make openapi
```

This requires the `utoipa` crate and Bridge to be built.

---

## Viewing Documentation

Import `openapi.json` into:

- Swagger UI
- Redoc
- Postman
- Insomnia

Or use online viewers:

```
https://petstore.swagger.io/?url=https://your-domain.com/openapi.json
```

---

## See Also

- [API Reference](../api-reference/index.md)
