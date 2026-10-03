# Quorum Dashboard Design

## Visual Direction

Frutiger Aero for a calm verification workspace: clear sky, water-toned surfaces, rounded functional controls, and restrained luminous accents. The mood is a bright operations room after rain, not a retro desktop imitation.

## Color System

Strategy: restrained product UI with a committed aqua navigation surface.

```css
:root {
  --bg: oklch(0.985 0.008 205);
  --surface: oklch(1 0 0);
  --surface-cool: oklch(0.955 0.025 205);
  --ink: oklch(0.24 0.035 220);
  --muted: oklch(0.48 0.035 220);
  --primary: oklch(0.55 0.145 150);
  --aqua: oklch(0.64 0.13 205);
  --accent: oklch(0.56 0.16 255);
  --success: oklch(0.52 0.15 150);
  --warning: oklch(0.68 0.15 82);
  --danger: oklch(0.57 0.2 28);
}
```

## Typography

Use `Inter, ui-sans-serif, system-ui, sans-serif` for UI and data. Use a fixed, compact scale. Use the system monospace stack for digests, IDs, and commits.

## Components

- Top bar, compact navigation rail, page tabs, decision/status pills, evidence rows, data tables, alert banners, skeleton loaders, and inline empty states.
- Controls have visible keyboard focus, 180ms state transitions, and white text on saturated fills.
- Status always pairs color with icon and text.

## Responsive Behavior

Desktop uses a navigation rail and information-dense evidence workspace. Tablet collapses rail labels. Mobile turns the rail into a horizontal scrollable nav and stacks verification panels.

## Motion

Use only state transitions and loading shimmer. Respect `prefers-reduced-motion: reduce` by disabling transitions and animation.
