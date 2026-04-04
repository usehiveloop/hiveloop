import { describe, it, expect } from "vitest";
import { vault } from "./setup.js";

describe("api-keys", () => {
  let createdKeyId: string;

  it("creates an api key", async () => {
    const { data, error } = await vault.apiKeys.create({
      name: `sdk-test-${Date.now()}`,
      scopes: ["credentials"],
    });

    expect(error).toBeUndefined();
    expect(data).toBeDefined();
    expect(data!.id).toBeDefined();
    expect(data!.key).toBeDefined();
    expect(data!.key!.startsWith("zira_sk_")).toBe(true);

    createdKeyId = data!.id!;
  });

  it("lists api keys", async () => {
    const { data, error } = await vault.apiKeys.list({ limit: 5 });

    expect(error).toBeUndefined();
    expect(data).toBeDefined();
    expect(Array.isArray(data!.data)).toBe(true);
    expect(typeof data!.has_more).toBe("boolean");
  });

  it("deletes the created api key", async () => {
    const { data, error } = await vault.apiKeys.delete(createdKeyId);

    expect(error).toBeUndefined();
    expect(data).toBeDefined();
  });
});
