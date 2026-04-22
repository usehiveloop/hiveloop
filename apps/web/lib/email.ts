export function localPart(email?: string | null): string {
  if (!email) return ""
  const at = email.indexOf("@")
  return at > 0 ? email.slice(0, at) : email
}
