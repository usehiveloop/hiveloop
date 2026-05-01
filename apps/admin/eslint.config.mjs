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
    // Logging hygiene. Use a structured logger instead of raw console for
    // anything operational. console.warn/console.error are tolerated as
    // last-resort escape hatches; everything else is a build failure.
    rules: {
      "no-console": ["error", { allow: ["warn", "error"] }],
    },
  },
]);

export default eslintConfig;
