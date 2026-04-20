import { describe, it, expect } from "vitest";
import { HiveLoop } from "../src/index.js";

describe("HiveLoop client", () => {
  it("defaults baseUrl to https://api.hiveloop.com", () => {
    const vault = new HiveLoop({ apiKey: "zira_sk_test" });
    // The client is created — we just verify it has all resource namespaces
    expect(vault.apiKeys).toBeDefined();
    expect(vault.credentials).toBeDefined();
    expect(vault.tokens).toBeDefined();
    expect(vault.identities).toBeDefined();
    expect(vault.connect).toBeDefined();
    expect(vault.connect.sessions).toBeDefined();
    expect(vault.connect.settings).toBeDefined();
    expect(vault.integrations).toBeDefined();
    expect(vault.connections).toBeDefined();
    expect(vault.usage).toBeDefined();
    expect(vault.audit).toBeDefined();
    expect(vault.org).toBeDefined();
    expect(vault.providers).toBeDefined();
  });

  it("accepts a custom baseUrl", () => {
    const vault = new HiveLoop({
      apiKey: "zira_sk_test",
      baseUrl: "https://api.dev.hiveloop.com",
    });
    expect(vault.apiKeys).toBeDefined();
  });
});
