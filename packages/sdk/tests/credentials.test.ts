import { describe, it, expect } from "vitest";
import { vault } from "./setup.js";

describe("credentials", () => {
  let createdId: string;

  it("creates a credential", async () => {
    const { data, error } = await vault.credentials.create({
      label: `sdk-test-${Date.now()}`,
      provider_id: "openai",
      base_url: "https://api.openai.com/v1",
      auth_scheme: "bearer",
      api_key: "sk-test-fake-key-for-sdk-tests",
    });

    expect(error).toBeUndefined();
    expect(data).toBeDefined();
    expect(data!.id).toBeDefined();
    expect(data!.provider_id).toBe("openai");

    createdId = data!.id!;
  });

  it("lists credentials", async () => {
    const { data, error } = await vault.credentials.list({ limit: 5 });

    expect(error).toBeUndefined();
    expect(data).toBeDefined();
    expect(Array.isArray(data!.data)).toBe(true);
    expect(typeof data!.has_more).toBe("boolean");
  });

  it("deletes the created credential", async () => {
    const { data, error } = await vault.credentials.delete(createdId);

    expect(error).toBeUndefined();
    expect(data).toBeDefined();
  });
});
