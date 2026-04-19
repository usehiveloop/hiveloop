export function extractErrorMessage(error: unknown, fallback: string, context: string): string {
  if (error && typeof error === "object" && "error" in error) {
    const message = (error as { error?: string }).error
    if (typeof message === "string" && message.length > 0) {
      return context ? `${context}: ${message}` : message
    }
  }
  return fallback
}
