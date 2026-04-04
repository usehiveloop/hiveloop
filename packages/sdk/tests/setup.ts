import dotenv from "dotenv";
import path from "path";
import { ZiraLoop } from "../src/index.js";

dotenv.config({ path: path.resolve(__dirname, "../../../.env") });

const apiKey = process.env.ZIRALOOP_API_KEY;
if (!apiKey) {
  throw new Error("ZIRALOOP_API_KEY is required in .env");
}

export const vault = new ZiraLoop({
  apiKey,
  baseUrl: "https://api.dev.ziraloop.com",
});
