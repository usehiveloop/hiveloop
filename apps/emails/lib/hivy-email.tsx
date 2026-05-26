import type { CSSProperties, ReactNode } from "react"
import {
  Body,
  Button,
  Container,
  Head,
  Heading,
  Hr,
  Html,
  Img,
  Link,
  Preview,
  Section,
  Text,
} from "@react-email/components"

export const siteUrl = "https://usehivy.com"

const emailAssetBaseUrl =
  process.env.HIVY_EMAIL_RENDER_TARGET === "resend" || process.env.NODE_ENV === "production"
    ? (process.env.HIVY_EMAIL_ASSET_URL ?? `${siteUrl}/email`)
    : "/static"

export const brand = {
  background: "#FFFAFA",
  foreground: "#2D0A0F",
  card: "#FFFFFF",
  primary: "#881337",
  primaryForeground: "#FFFFFF",
  muted: "#FFF1F2",
  mutedForeground: "#7C4A52",
  accent: "#FB7185",
  border: "#FECDD3",
  logo: `${emailAssetBaseUrl}/hivy-logo-dark.png`,
}

const legalAddress =
  "HIVY TECHNOLOGIES LTD, B1 Orji Murray St, behind Romay Garden Estate, off Lekki - Epe Expressway, Eti-Osa, Lekki 106104, Lagos."

type HivyEmailProps = {
  preview: string
  eyebrow: string
  title: string
  children: ReactNode
  footerNote?: string
}

export function HivyEmail({ preview, eyebrow, title, children, footerNote }: HivyEmailProps) {
  return (
    <Html lang="en">
      <Head />
      <Preview>{preview}</Preview>
      <Body style={styles.body}>
        <Container style={styles.container}>
          <Section style={styles.header}>
            <Link href={siteUrl} style={styles.logoLink}>
              <Img src={brand.logo} width="118" height="48" alt="Hivy" style={styles.logo} />
            </Link>
          </Section>

          <Section style={styles.card}>
            <Text style={styles.eyebrow}>{eyebrow}</Text>
            <Heading as="h1" style={styles.heading}>
              {title}
            </Heading>
            {children}
          </Section>

          <Section style={styles.footer}>
            {footerNote ? <Text style={styles.footerText}>{footerNote}</Text> : null}
            <Text style={styles.footerText}>
              <Link href={siteUrl} style={styles.footerLink}>
                usehivy.com
              </Link>
              {"  |  "}
              <Link href={`${siteUrl}/terms`} style={styles.footerLink}>
                Terms
              </Link>
              {"  |  "}
              <Link href={`${siteUrl}/legal`} style={styles.footerLink}>
                Privacy
              </Link>
            </Text>
            <Text style={styles.address}>{legalAddress}</Text>
          </Section>
        </Container>
      </Body>
    </Html>
  )
}

export function Paragraph({ children }: { children: ReactNode }) {
  return <Text style={styles.paragraph}>{children}</Text>
}

export function Detail({ label, value }: { label: string; value: ReactNode }) {
  return (
    <Section style={styles.detailRow}>
      <Text style={styles.detailLabel}>{label}</Text>
      <Text style={styles.detailValue}>{value}</Text>
    </Section>
  )
}

export function CodePanel({ code }: { code: string }) {
  return (
    <Section style={styles.codePanel}>
      <Text style={styles.codeText}>{code}</Text>
    </Section>
  )
}

export function PrimaryButton({ href, children }: { href: string; children: ReactNode }) {
  return (
    <Button href={href} style={styles.button}>
      {children}
    </Button>
  )
}

export function Divider() {
  return <Hr style={styles.divider} />
}

const fontFamily = "Arial, Helvetica, sans-serif"
const headingFont = "Georgia, 'Times New Roman', serif"

const styles: Record<string, CSSProperties> = {
  body: {
    margin: 0,
    backgroundColor: brand.background,
    color: brand.foreground,
    fontFamily,
  },
  container: {
    width: "100%",
    maxWidth: "600px",
    margin: "0 auto",
    padding: "32px 20px",
  },
  header: {
    padding: "0 0 18px",
  },
  logoLink: {
    display: "inline-block",
  },
  logo: {
    display: "block",
    border: 0,
    outline: "none",
    textDecoration: "none",
  },
  card: {
    backgroundColor: brand.card,
    border: `1px solid ${brand.border}`,
    borderRadius: "8px",
    padding: "32px",
  },
  eyebrow: {
    margin: "0 0 12px",
    color: brand.primary,
    fontSize: "13px",
    fontWeight: 700,
    lineHeight: "20px",
  },
  heading: {
    margin: "0 0 20px",
    color: brand.foreground,
    fontFamily: headingFont,
    fontSize: "30px",
    fontWeight: 600,
    lineHeight: "36px",
    letterSpacing: 0,
  },
  paragraph: {
    margin: "0 0 16px",
    color: brand.foreground,
    fontSize: "16px",
    lineHeight: "26px",
  },
  codePanel: {
    margin: "24px 0",
    padding: "22px 24px",
    backgroundColor: brand.muted,
    border: `1px solid ${brand.border}`,
    borderRadius: "8px",
    textAlign: "center",
  },
  codeText: {
    margin: 0,
    color: brand.primary,
    fontFamily: "'SFMono-Regular', Consolas, 'Liberation Mono', monospace",
    fontSize: "34px",
    fontWeight: 700,
    lineHeight: "42px",
    letterSpacing: 0,
  },
  button: {
    display: "inline-block",
    boxSizing: "border-box",
    margin: "10px 0 20px",
    padding: "13px 18px",
    backgroundColor: brand.primary,
    borderRadius: "6px",
    color: brand.primaryForeground,
    fontSize: "15px",
    fontWeight: 700,
    lineHeight: "20px",
    textDecoration: "none",
    textAlign: "center",
  },
  detailRow: {
    margin: "18px 0",
    padding: "14px 16px",
    backgroundColor: brand.muted,
    border: `1px solid ${brand.border}`,
    borderRadius: "8px",
  },
  detailLabel: {
    margin: "0 0 4px",
    color: brand.mutedForeground,
    fontSize: "12px",
    fontWeight: 700,
    lineHeight: "18px",
  },
  detailValue: {
    margin: 0,
    color: brand.foreground,
    fontSize: "15px",
    lineHeight: "22px",
  },
  divider: {
    margin: "24px 0",
    border: "none",
    borderTop: `1px solid ${brand.border}`,
  },
  footer: {
    padding: "24px 28px 0",
    textAlign: "center",
  },
  footerText: {
    margin: "0 auto 8px",
    color: brand.mutedForeground,
    fontSize: "12px",
    lineHeight: "18px",
    textAlign: "center",
  },
  footerLink: {
    color: brand.primary,
    textDecoration: "underline",
  },
  address: {
    margin: "0 auto",
    color: brand.mutedForeground,
    fontSize: "11px",
    lineHeight: "17px",
    maxWidth: "430px",
    textAlign: "center",
  },
}
