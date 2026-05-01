import { defineConfig, globalIgnores } from "eslint/config";
import nextVitals from "eslint-config-next/core-web-vitals";
import nextTs from "eslint-config-next/typescript";

const eslintConfig = defineConfig([
  ...nextVitals,
  ...nextTs,
  // Override default ignores of eslint-config-next.
  globalIgnores([
    // Default ignores of eslint-config-next:
    ".next/**",
    "out/**",
    "build/**",
    "next-env.d.ts",
  ]),
  {
    // Logging hygiene. Use the pino logger from @/lib/logger instead of
    // raw console for anything operational. console.warn/console.error are
    // tolerated as last-resort escape hatches; everything else is a build
    // failure. Sentry's @sentry/nextjs client auto-captures unhandled
    // errors, so most "log this somewhere" needs are already covered.
    rules: {
      "no-console": ["error", { allow: ["warn", "error"] }],
    },
  },
  {
    // Build/codegen scripts are CLI-shaped — console output is the point.
    files: ["scripts/**/*.{ts,mts,tsx,js,mjs}"],
    rules: {
      "no-console": "off",
    },
  },
]);

export default eslintConfig;
