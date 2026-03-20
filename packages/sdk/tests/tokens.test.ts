import { describe, it, expect } from "vitest";
import { vault } from "./setup.js";

describe("tokens", () => {
  let credentialId: string;
  let createdJti: string;

  it("creates a token for a credential", async () => {
    // First create a credential to mint a token for
    const { data: cred } = await vault.credentials.create({
      label: `sdk-token-test-${Date.now()}`,
      provider_id: "openai",
      base_url: "https://api.openai.com/v1",
      auth_scheme: "bearer",
      api_key: "sk-test-fake-key-for-token-tests",
    });
    credentialId = cred!.id!;

    const { data, error } = await vault.tokens.create({
      credential_id: credentialId,
      ttl: "1h",
    });

    expect(error).toBeUndefined();
    expect(data).toBeDefined();
    expect(data!.token).toBeDefined();
    expect(data!.token!.startsWith("ptok_")).toBe(true);
    expect(data!.jti).toBeDefined();

    createdJti = data!.jti!;
  });

  it("deletes the created token", async () => {
    const { data, error } = await vault.tokens.delete(createdJti);

    expect(error).toBeUndefined();
    expect(data).toBeDefined();
  });

  // Cleanup
  it("cleans up the test credential", async () => {
    await vault.credentials.delete(credentialId);
  });
});
