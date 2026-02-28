# UI Design Spec — Babylon Tower

## Concept

A Babylonian-themed UI with parallax depth:
- **Bottom of screen:** city view (chat, contacts)
- **Scrolling up:** sky and the Tower of Babel rising above
- **Light mode:** daylight scene
- **Dark mode:** night scene

## Color Palette

### Light Mode (Daylight Babylon)

| Role | Color | Hex | Inspiration |
|------|-------|-----|-------------|
| Primary | Lapis lazuli blue | `#1E3A5F` | Ishtar Gate glazed bricks |
| Primary variant | Turquoise glaze | `#2E8B8B` | Babylonian ceramic tiles |
| Accent/gold | Hammered gold | `#C5961A` | Jewelry, ziggurat trim |
| Background (city) | Warm sandstone | `#E8D5B7` | Mud-brick architecture |
| Background (sky) | Desert sky blue | `#87CEEB` → `#F5E6CA` | Gradient, hazy horizon |
| Surface/cards | Pale clay | `#F2E8D5` | Tablet/papyrus feel |
| Text primary | Dark umber | `#2C1810` | Burnt clay inscription |
| Text secondary | Warm grey | `#6B5B4F` | Weathered stone |
| Success/online | Palm green | `#4A7C3F` | Hanging Gardens foliage |
| Error/alert | Terracotta red | `#B5453A` | Fired brick |

### Dark Mode (Night Babylon)

| Role | Color | Hex | Inspiration |
|------|-------|-----|-------------|
| Primary | Bright lapis | `#4A7FBF` | Ishtar Gate under moonlight |
| Primary variant | Teal glaze | `#3AAFAF` | Ceramic glow |
| Accent/gold | Warm gold | `#D4A843` | Torchlit gold leaf |
| Background (city) | Deep brown-black | `#1A1410` | Night mud-brick |
| Background (sky) | Night sky | `#0A0E1A` → `#1A1410` | Starfield to horizon |
| Surface/cards | Dark clay | `#2A2218` | Shadow on sandstone |
| Text primary | Pale sand | `#E8D5B7` | Moonlit stone |
| Text secondary | Muted sand | `#8B7D6B` | Dim inscription |
| Success/online | Soft green | `#5A9C4F` | Garden moonlight |
| Error/alert | Warm red | `#CF6B5A` | Ember glow |

## Visual Assets

### City Layer (bottom — fixed/slow parallax)
- Silhouette of low Babylonian buildings (flat roofs, arched doorways)
- Palm trees scattered between structures
- Ishtar Gate as a recognizable landmark element
- Torches/lanterns for dark mode (warm point lights)

### Tower (middle — medium parallax)
- The ziggurat/tower rising from city center
- Stepped terraces with hanging garden vegetation
- Gold trim on upper tiers catching light
- Night mode: window lights, torch glow on tiers

### Sky Layer (top — fast parallax)
- Light: warm desert sky with subtle clouds, distant sun
- Dark: deep starfield — Babylonians were astronomers, so constellations are thematic
- Optional: crescent moon (the moon god Sin was major in Babylon)

### UI Elements
- Cuneiform-inspired decorative borders (subtle, not overdone)
- Message bubbles with slightly rounded-rectangle "clay tablet" shape
- Icons with simplified Babylonian motifs (winged bull for settings, palm for contacts, ziggurat for home)
- Scrollbar styled as a column or rope

## Implementation Notes

- Keep parallax layers as **SVG or layered PNG** for crisp scaling
- The city silhouette works well as a single repeating vector strip
- Use the gold accent sparingly — buttons, active tabs, unread badges
- Sandstone/clay textures should be very subtle (5-10% opacity overlay) to keep text readable
- Stars in dark mode can be a simple CSS/canvas animation
- The **Ishtar Gate blue + gold** combination is the signature duo
