---
title: Theming Connect
description: Customize the appearance of the LLMVault Connect widget with theme modes, custom colors, typography, and border radius.
---

# Theming Connect

LLMVault Connect supports extensive customization to match your brand's visual identity. You can customize colors, typography, border radius, and theme modes.

## Theme Modes

Connect supports three theme modes that control the overall color scheme:

```typescript
import { LLMVaultConnect } from '@llmvault/frontend';

const connect = new LLMVaultConnect({
  theme: 'light',  // 'light' | 'dark' | 'system'
});
```

| Mode | Description |
|------|-------------|
| `light` | Light background with dark text |
| `dark` | Dark background with light text |
| `system` | Follows user's OS preference (default) |

### System Theme Detection

When `theme: 'system'` is set, Connect automatically detects the user's preference:

```typescript
// Uses window.matchMedia('(prefers-color-scheme: dark)')
const systemTheme = window.matchMedia('(prefers-color-scheme: dark)').matches 
  ? 'dark' 
  : 'light';
```

The widget updates dynamically if the user changes their OS theme while the widget is open.

## Custom Colors

Pass custom colors via URL parameters to override the default palette. Colors should be provided as hex values **without the `#` prefix**.

### URL Parameter Method

```typescript
const connect = new LLMVaultConnect({
  theme: 'light',
});

// Colors are passed automatically when the iframe loads
// The SDK appends them as URL search params
```

### Available Color Parameters

| Parameter | CSS Variable | Default (Light) | Description |
|-----------|--------------|-----------------|-------------|
| `accent` | `--color-cw-accent` | `#8B5CF6` | Primary brand color |
| `bg` | `--color-cw-bg` | `#FFFFFF` | Widget background |
| `surface` | `--color-cw-surface` | `#F9FAFB` | Card/surface background |
| `border` | `--color-cw-border` | `#E4E4E7` | Border color |
| `heading` | `--color-cw-heading` | `#09090B` | Primary text (headings) |
| `body` | `--color-cw-body` | `#71717A` | Secondary text (body) |
| `success` | `--color-cw-success` | `#22C55E` | Success states |
| `error` | `--color-cw-error` | `#EF4444` | Error states |

### Color Derivatives

When you set certain colors, Connect automatically generates derivative colors:

**Accent derivatives:**
- `--color-cw-accent-hover` - Darkened 12%
- `--color-cw-accent-active` - Darkened 22%
- `--color-cw-accent-subtle` - 6% opacity
- `--color-cw-accent-subtle-border` - 12% opacity
- `--color-cw-logo` - Same as accent

**Success derivatives:**
- `--color-cw-success-bg` - 10% opacity background

**Error derivatives:**
- `--color-cw-error-bg` - 10% opacity background
- `--color-cw-error-hover` - Darkened 15%

### Complete Color Example

```typescript
// Initialize with your brand colors
const connect = new LLMVaultConnect({
  theme: 'light',
});

// The widget automatically uses these CSS custom properties
// which you can override via URL parameters:
//
// ?accent=6366F1      // Indigo brand color
// &bg=FFFFFF          // White background
// &surface=F8FAFC     // Light gray surface
// &border=E2E8F0      // Slate border
// &heading=0F172A     // Dark slate headings
// &body=64748B        // Slate body text
// &success=10B981     // Emerald success
// &error=EF4444       // Red error
```

### Dark Mode Colors

When the widget is in dark mode, it uses a different default palette:

| Variable | Default (Dark) |
|----------|----------------|
| `--color-cw-bg` | `#1C1B22` |
| `--color-cw-surface` | `#16151B` |
| `--color-cw-border` | `#2A2932` |
| `--color-cw-heading` | `#F5F5F4` |
| `--color-cw-body` | `#A1A1AA` |
| `--color-cw-accent-subtle` | `#8B5CF614` (8% opacity) |

### Brand Color Examples

```typescript
// Indigo brand
connect.open({
  sessionToken: 'sess_xxx',
  // accent=6366F1
});

// Blue brand  
connect.open({
  sessionToken: 'sess_xxx',
  // accent=3B82F6
});

// Emerald brand
connect.open({
  sessionToken: 'sess_xxx',
  // accent=10B981
});

// Rose brand
connect.open({
  sessionToken: 'sess_xxx',
  // accent=F43F5E
});
```

## Typography

Customize the font family to match your brand:

### URL Parameter

| Parameter | CSS Property | Default |
|-----------|--------------|---------|
| `font` | `font-family` | `'Inter', system-ui, sans-serif` |

### Font Examples

```typescript
// Use your custom font (must be loaded by your application)
connect.open({
  sessionToken: 'sess_xxx',
  // font=Your+Font+Name
});
```

The widget will apply:

```css
.connect-widget {
  font-family: 'Your Font Name', system-ui, sans-serif;
}
```

### Loading Custom Fonts

Ensure your custom font is loaded in the parent page before opening Connect:

```html
<link href="https://fonts.googleapis.com/css2?family=Your+Font:wght@400;500;600;700&display=swap" rel="stylesheet">
```

```css
/* Or via @font-face */
@font-face {
  font-family: 'Your Font';
  src: url('/fonts/your-font.woff2') format('woff2');
  font-weight: 400 700;
  font-display: swap;
}
```

### Footer Logo Font

The footer logo uses a distinct font:

```css
.connect-logo-text {
  font-family: 'Bricolage Grotesque', system-ui, sans-serif;
}
```

## Border Radius

Customize the corner roundness of the widget:

### URL Parameter

| Parameter | CSS Property | Default |
|-----------|--------------|---------|
| `radius` | `border-radius` | `12px` (desktop) |

### Radius Values

Valid formats: `px`, `rem`, `em`

```typescript
// Sharp corners
connect.open({
  sessionToken: 'sess_xxx',
  // radius=0px
});

// Slightly rounded
connect.open({
  sessionToken: 'sess_xxx',
  // radius=8px
});

// Very rounded
connect.open({
  sessionToken: 'sess_xxx',
  // radius=24px
});

// Pill-shaped (for specific elements)
connect.open({
  sessionToken: 'sess_xxx',
  // radius=9999px
});
```

### Border Radius Examples

| Value | Appearance |
|-------|------------|
| `0px` | Sharp, rectangular |
| `8px` | Subtle rounding |
| `12px` | Balanced (default) |
| `16px` | Prominent rounding |
| `24px` | Very soft |

## Responsive Behavior

Connect automatically adapts to different screen sizes:

### Desktop (≥480px)

```
┌──────────────────────────────┐
│                              │
│      Modal-style widget      │
│      480px × 680px           │
│      border-radius: 12px     │
│                              │
└──────────────────────────────┘
```

**Desktop-specific styles:**
- Widget: `480px × 680px`
- Border radius: `12px`
- Box shadow: `0 25px 50px -12px rgba(0, 0, 0, 0.25)`
- Padding: `28px`

### Mobile (<480px)

```
┌─────────────────────────────────┐
│                                 │
│    Full-screen overlay          │
│    100% × 100%                  │
│                                 │
└─────────────────────────────────┘
```

**Mobile-specific styles:**
- Widget: `100% × 100%`
- No border radius (full screen)
- Padding: `48px 24px 24px`

### Responsive Typography

| Element | Desktop | Mobile |
|---------|---------|--------|
| Title | `font-bold` (700) | `font-semibold` (600) |
| Provider buttons | `rounded-lg` | `rounded-full` (popular) |
| Input fields | `rounded-lg` | `rounded-2.5` (10px) |
| Connect button | `rounded-lg` | `rounded-2.5` (10px) |

## Provider Colors

Each LLM provider has a dedicated brand color used for their logo:

| Provider | CSS Variable | Color |
|----------|--------------|-------|
| OpenAI | `--color-cw-provider-openai` | `#0D0D0D` |
| Anthropic | `--color-cw-provider-anthropic` | `#D4A373` |
| Google Gemini | `--color-cw-provider-google-gemini` | `#4285F4` |
| Mistral | `--color-cw-provider-mistral` | `#4A90D9` |
| Groq | `--color-cw-provider-groq` | `#E8734A` |
| DeepSeek | `--color-cw-provider-deepseek` | `#4A6CF7` |
| Cohere | `--color-cw-provider-cohere` | `#39594D` |
| Perplexity | `--color-cw-provider-perplexity` | `#6366F1` |

These colors are used in the provider selection list and cannot be overridden.

## Complete Theming Example

```typescript
import { LLMVaultConnect } from '@llmvault/frontend';

// Brand configuration
const brandConfig = {
  light: {
    accent: '6366F1',      // Indigo
    bg: 'FFFFFF',
    surface: 'F8FAFC',
    border: 'E2E8F0',
    heading: '0F172A',
    body: '64748B',
    success: '10B981',
    error: 'EF4444',
    radius: '12px',
    font: 'Inter',
  },
  dark: {
    accent: '818CF8',      // Lighter indigo for dark
    bg: '0F172A',
    surface: '1E293B',
    border: '334155',
    heading: 'F8FAFC',
    body: '94A3B8',
    success: '34D399',
    error: 'F87171',
    radius: '12px',
    font: 'Inter',
  },
};

function openConnect(sessionToken: string, prefersDark: boolean) {
  const connect = new LLMVaultConnect({
    theme: prefersDark ? 'dark' : 'light',
  });
  
  connect.open({
    sessionToken,
    onSuccess: (payload) => {
      console.log('Connected:', payload);
    },
  });
}
```

## Testing Themes

You can preview themes using URL parameters directly:

```
https://connect.llmvault.dev?preview=true&theme=light&accent=6366F1&bg=FFFFFF
https://connect.llmvault.dev?preview=true&theme=dark&accent=818CF8&bg=0F172A
```

Set `preview=true` to bypass session validation for testing.

## Best Practices

1. **Accessibility**: Ensure sufficient contrast ratios (WCAG 4.5:1 for normal text)
2. **Consistency**: Match your application's color palette
3. **Testing**: Preview both light and dark modes
4. **Font Loading**: Always load custom fonts before opening Connect
5. **Brand Recognition**: Keep provider brand colors unchanged

## Next Steps

- [Embedding](./embedding) — Learn how to embed the themed widget
- [Frontend SDK](./frontend-sdk) — Complete SDK reference
- [Sessions](./sessions) — Secure session management
