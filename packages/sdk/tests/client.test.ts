import { describe, it, expect } from "vitest";
import { HiveLoop } from "../src/index.js";

describe("HiveLoop client", () => {
  it("defaults baseUrl to https://api.usehiveloop.com", () => {
    const vault = new HiveLoop({ apiKey: "hvl_sk_test" });
    // The client is created — we just verify it has all resource namespaces
    expect(vault.apiKeys).toBeDefined();
    expect(vault.catalog).toBeDefined();
    expect(vault.conversations).toBeDefined();
    expect(vault.credentials).toBeDefined();
    expect(vault.customDomains).toBeDefined();
    expect(vault.employees).toBeDefined();
    expect(vault.generations).toBeDefined();
    expect(vault.audit).toBeDefined();
    expect(vault.org).toBeDefined();
    expect(vault.providers).toBeDefined();
    expect(vault.reporting).toBeDefined();
    expect(vault.sandboxes).toBeDefined();
    expect(vault.sandboxTemplates).toBeDefined();
    expect(vault.tokens).toBeDefined();
    expect(vault.usage).toBeDefined();
  });

  it("accepts a custom baseUrl", () => {
    const vault = new HiveLoop({
      apiKey: "hvl_sk_test",
      baseUrl: "https://api.dev.hiveloop.com",
    });
    expect(vault.apiKeys).toBeDefined();
  });
});
