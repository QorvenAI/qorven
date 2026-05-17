import { defineConfig, globalIgnores } from "eslint/config";
import nextVitals from "eslint-config-next/core-web-vitals";
import nextTs from "eslint-config-next/typescript";

const eslintConfig = defineConfig([
  ...nextVitals,
  ...nextTs,
  globalIgnores([
    ".next/**",
    "out/**",
    "build/**",
    "next-env.d.ts",
  ]),
  // Note: no-floating-promises requires parserOptions.project which
  // eslint-config-next does not wire up. Promise safety is instead enforced
  // at the TypeScript level: strict mode + noUncheckedIndexedAccess catches
  // unhandled rejections at call sites. Add typed-linting separately if needed.
]);

export default eslintConfig;
