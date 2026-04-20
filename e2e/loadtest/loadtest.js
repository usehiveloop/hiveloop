// k6 load test: 100 concurrent conversations across 5 agents
// Each conversation: create → open 5 SSE streams → 3 turn-by-turn messages → verify
//
// Usage: k6 run --env API_KEY=... --env AGENT_IDS=id1,id2,id3,id4,id5 loadtest.js

import http from "k6/http";
import { check, sleep, group } from "k6";
import { Counter, Trend, Rate } from "k6/metrics";

// --- Config ---
const BASE_URL = "https://api.usehiveloop.com";
const API_KEY = __ENV.API_KEY;
const AGENT_IDS = (__ENV.AGENT_IDS || "").split(",");

if (!API_KEY || AGENT_IDS.length === 0 || AGENT_IDS[0] === "") {
  throw new Error("Set API_KEY and AGENT_IDS env vars");
}

const HEADERS = {
  Authorization: `Bearer ${API_KEY}`,
  "Content-Type": "application/json",
};

const MESSAGES = [
  ["What is 7 * 8?", "What is the capital of Japan?", "How do I boil an egg?", "Best exercise for core?", "Who built the pyramids?"],
  ["Is pi rational?", "Largest country by area?", "Quick pasta recipe?", "How many pushups daily?", "When was Rome founded?"],
  ["What is a prime number?", "Where is Madagascar?", "How to make toast?", "Best stretching routine?", "Who was Cleopatra?"],
];

// --- Custom metrics ---
const conversationCreated = new Counter("conversations_created");
const messagesAccepted = new Counter("messages_accepted");
const turnsCompleted = new Counter("turns_completed");
const streamsOpened = new Counter("streams_opened");
const streamsFailed = new Counter("streams_failed");
const conversationLatency = new Trend("conversation_create_ms");
const messageLatency = new Trend("message_send_ms");
const turnLatency = new Trend("turn_complete_ms");
const errorRate = new Rate("errors");

// --- k6 options ---
export const options = {
  scenarios: {
    load_test: {
      executor: "per-vu-iterations",
      vus: 200,
      iterations: 1,
      maxDuration: "15m",
    },
  },
  thresholds: {
    errors: ["rate<0.1"],          // <10% error rate
    conversation_create_ms: ["p(95)<120000"],  // 95th percentile < 120s
    message_send_ms: ["p(95)<5000"],           // 95th percentile < 5s
  },
};

export default function () {
  const vuId = __VU;
  const agentId = AGENT_IDS[vuId % AGENT_IDS.length];

  group("conversation_lifecycle", function () {
    // --- Step 1: Create conversation ---
    const createStart = Date.now();
    const createResp = http.post(
      `${BASE_URL}/v1/agents/${agentId}/conversations`,
      "{}",
      { headers: HEADERS, timeout: "180s" }
    );

    const createOk = check(createResp, {
      "conversation created": (r) => r.status === 200 || r.status === 201,
      "has conversation id": (r) => {
        try {
          return JSON.parse(r.body).id !== undefined;
        } catch {
          return false;
        }
      },
    });

    if (!createOk) {
      errorRate.add(1);
      console.error(`VU ${vuId}: Failed to create conversation: ${createResp.status} ${createResp.body}`);
      return;
    }

    const conv = JSON.parse(createResp.body);
    const convId = conv.id;
    conversationCreated.add(1);
    conversationLatency.add(Date.now() - createStart);
    console.log(`VU ${vuId}: Created conversation ${convId} on agent ${agentId}`);

    // --- Step 2: Open 5 SSE stream connections ---
    // k6 doesn't natively support SSE long-polling, so we verify the endpoint responds
    // and test basic connectivity. For true SSE fan-out, we verify via the events API after.
    for (let s = 0; s < 5; s++) {
      const streamResp = http.get(
        `${BASE_URL}/v1/conversations/${convId}/stream`,
        { headers: { Authorization: `Bearer ${API_KEY}` }, timeout: "5s" }
      );
      if (streamResp.status === 200) {
        streamsOpened.add(1);
      } else {
        streamsFailed.add(1);
      }
    }

    // --- Step 3: Send 3 messages turn-by-turn ---
    const msgSet = vuId % MESSAGES.length;
    for (let turn = 0; turn < 3; turn++) {
      const msgContent = MESSAGES[turn][vuId % MESSAGES[turn].length];

      const sendStart = Date.now();
      const sendResp = http.post(
        `${BASE_URL}/v1/conversations/${convId}/messages`,
        JSON.stringify({ content: msgContent }),
        { headers: HEADERS, timeout: "30s" }
      );

      const sendOk = check(sendResp, {
        "message accepted": (r) => r.status === 200 || r.status === 202,
      });

      if (sendOk) {
        messagesAccepted.add(1);
        messageLatency.add(Date.now() - sendStart);
      } else {
        errorRate.add(1);
        console.error(`VU ${vuId}: Message failed: ${sendResp.status} ${sendResp.body}`);
        break; // Don't continue turns if one fails
      }

      // Wait for turn to complete by polling events API
      const turnStart = Date.now();
      let turnDone = false;
      for (let poll = 0; poll < 30; poll++) {
        sleep(2);
        const eventsResp = http.get(
          `${BASE_URL}/v1/conversations/${convId}/events`,
          { headers: { Authorization: `Bearer ${API_KEY}` }, timeout: "10s" }
        );

        if (eventsResp.status === 200) {
          try {
            const events = JSON.parse(eventsResp.body);
            const completed = (events.data || []).filter(
              (e) => e.event_type === "turn_completed"
            ).length;
            if (completed >= turn + 1) {
              turnDone = true;
              turnsCompleted.add(1);
              turnLatency.add(Date.now() - turnStart);
              console.log(`VU ${vuId}: Turn ${turn + 1} completed in ${Date.now() - turnStart}ms`);
              break;
            }
          } catch {}
        }
      }

      if (!turnDone) {
        errorRate.add(1);
        console.error(`VU ${vuId}: Turn ${turn + 1} timed out after 60s`);
        break;
      }
    }

    // --- Step 4: Verify events persisted ---
    sleep(3); // Give flusher time
    const finalEvents = http.get(
      `${BASE_URL}/v1/conversations/${convId}/events`,
      { headers: { Authorization: `Bearer ${API_KEY}` }, timeout: "10s" }
    );

    check(finalEvents, {
      "events persisted": (r) => {
        try {
          const data = JSON.parse(r.body);
          return (data.data || []).length > 0;
        } catch {
          return false;
        }
      },
    });

    errorRate.add(0);
  });
}
