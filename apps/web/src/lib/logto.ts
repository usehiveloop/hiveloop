import { UserScope, type LogtoNextConfig } from "@logto/next";

function required(name: string): string {
  const value = process.env[name];
  if (!value) {
    throw new Error(`Missing required environment variable: ${name}`);
  }
  return value;
}

export function getLogtoConfig(): LogtoNextConfig {
  return {
    endpoint: required("LOGTO_ENDPOINT"),
    appId: required("LOGTO_APP_ID"),
    appSecret: required("LOGTO_APP_SECRET"),
    baseUrl: required("LOGTO_BASE_URL"),
    cookieSecret: required("LOGTO_COOKIE_SECRET"),
    cookieSecure: process.env.NODE_ENV === "production",
    scopes: [
      UserScope.Email,
      UserScope.Organizations,
      UserScope.OrganizationRoles,
    ],
    resources: [required("NEXT_PUBLIC_API_RESOURCE")],
  };
}
