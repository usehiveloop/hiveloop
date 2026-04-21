import dotenv from "dotenv";
import path from "path";
import { HiveLoop } from "../src/index.js";

dotenv.config({ path: path.resolve(__dirname, "../../../.env") });

const apiKey = process.env.HIVELOOP_API_KEY;
if (!apiKey) {
  throw new Error("HIVELOOP_API_KEY is required in .env");
}

export const vault = new HiveLoop({
  apiKey,
  baseUrl: "https://api.dev.hiveloop.com",
});
