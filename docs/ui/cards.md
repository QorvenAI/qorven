# Chat rendering — cards, tool calls, citations

Qorven's chat renders structured cards alongside the LLM's prose. This doc covers the full pipeline: what exists, how it's wired, and how to add a new card.

## Table of contents

- [Why cards](#why-cards)
- [Architecture](#architecture)
- [Card registry](#card-registry)
- [Adding a new card](#adding-a-new-card)
- [Emitting a card from a backend tool](#emitting-a-card-from-a-backend-tool)
- [Tool calls, thinking, citations](#tool-calls-thinking-citations)
- [Streaming caret](#streaming-caret)
- [Theming](#theming)

---

## why cards

Prose answers are fine for 90% of questions. For the other 10%, the user's intent produces structured data the LLM shouldn't have to turn into bullet lists: a weather forecast, a flight comparison, a SQL result, a screenshot from the agent's browser. Cards give those answers a first-class rendering that's easier to scan than markdown and easier to action than plain text.

Design principles:
1. **Cards complement, never replace.** Every card is emitted next to markdown text. If the user's client doesn't support the card type (CLI, Telegram, Slack text-only), the prose still answers the question.
2. **Self-contained data.** A card renders only what's in its data payload — no live fetches, no external APIs. The server does the lookup once; the card renders what it gets.
3. **One renderer per type, registered centrally.** `web/components/chat/part-renderer.tsx` is the single switch that dispatches widget types to components. New types land there.

---

## architecture

```
Backend                                           Frontend
┌────────────────────┐                            ┌──────────────────────┐
│ tool.Execute()     │                            │ chat-window.tsx      │
│  → *Result{Widget} │                            │  reads stream events │
└─────────┬──────────┘                            │  + message.parts[]   │
          │                                       └──────────┬───────────┘
          ▼                                                  │
┌────────────────────┐        WebSocket / SSE                ▼
│ agent.loop.go      │  ───────────────────────▶  ┌──────────────────────┐
│  WidgetEvent(w)    │    widget event +           │ MessageParts         │
│  WidgetPart(w)     │    parts[] on message       │  switch on part.type │
└────────────────────┘                             └──────────┬───────────┘
                                                              │
                                                              ▼
                                                   ┌──────────────────────┐
                                                   │ part-renderer.tsx    │
                                                   │  WidgetPartRenderer  │
                                                   │  → right Card        │
                                                   └──────────┬───────────┘
                                                              │
                                                              ▼
                                                   ┌──────────────────────┐
                                                   │ cards/weather-card   │
                                                   │ cards/travel-cards   │
                                                   │ cards/data-cards     │
                                                   │ cards/rich-cards     │
                                                   └──────────────────────┘
```

A backend tool fills `Result.Widget = &Widget{Type: "flights", Data: ...}`. The agent loop picks that up, emits a `widget` streaming event **and** a `widget` `MessagePart`. The frontend's part renderer looks up the type in its switch and renders the matching React card.

---

## card registry

Every card type lives in `web/components/chat/cards/`. The registry switch is in `web/components/chat/part-renderer.tsx` — `WidgetPartRenderer`. Current types:

### core (rich-cards.tsx)
`image`, `youtube`, `link`, `github`, `audio`, `video`, `diff`, `terminal`, `json`, `callout`, `quote`, `stat`, `map`, `comparison`, `timeline`, `task`, `calendar`, `file`, `soul`, `progress`

### weather (weather-card.tsx)
`weather`

### travel (travel-cards.tsx)
`flights` / `flight`, `hotels` / `hotel`, `cars` / `car` / `car_rental`, `transit` / `train` / `bus`

### data & ops (data-cards.tsx)
`sports_score` / `score`, `sql_result`, `screenshot`, `browser_step`

### artifact (artifact-preview.tsx)
Rendered by the `code` part type when the language is `html`, `svg`, or `mermaid` — these get an iframe preview with fullscreen toggle.

---

## adding a new card

1. **Pick a type name.** Lowercase, snake_case, descriptive. Avoid verbs — call it `stock_quote`, not `show_stock_quote`. If the type is a family with variants, use plural: `flights` takes an array.

2. **Decide the data shape.** Write an interface. Keep it flat; avoid nested optional trees. Prices should be `{amount, currency}` pairs so the card can format with `Intl.NumberFormat`.

3. **Build the component.** Drop it in the closest existing file (`data-cards.tsx` for operational data, `travel-cards.tsx` for verticals) or create a new file if the family is novel (sports, finance, etc.). Style guidelines — keep consistent with existing cards:
   - `my-2 rounded-xl border border-border bg-card/40 overflow-hidden`
   - Header: `px-4 py-2.5 border-b border-border/50 bg-muted/20` with a lucide icon + title + optional meta
   - Body: 8-row max, `divide-y divide-border/50` for lists, hover: `hover:bg-accent/20`
   - Text scale: titles `text-sm font-semibold`, body `text-xs`, meta `text-2xs text-muted-foreground`
   - Icons: `h-3.5 w-3.5` inline, `h-4 w-4` in headers

4. **Register the type.** Open `part-renderer.tsx`, add a `case` to `WidgetPartRenderer`:

   ```tsx
   case 'stock_quote': return <StockQuoteCard data={widgetData} />;
   ```

5. **Emit from a backend tool.** In Go, set the `Widget` field on your `Result`:

   ```go
   return &Result{
       ForLLM: humanSummary,  // markdown for the model/CLI
       ForUser: humanSummary,
       Widget: &Widget{
           Type: "stock_quote",
           Data: map[string]any{
               "symbol":    "AAPL",
               "price":     192.34,
               "change":    +0.82,
               "currency":  "USD",
           },
       },
   }
   ```

6. **Add to this doc.** Keep the registry list current.

---

## emitting a card from a backend tool

See `backend/internal/tools/weather.go` for the canonical pattern. The tool:

1. Does its lookup.
2. Builds a markdown summary (`ForLLM`/`ForUser`) — the LLM sees this as the tool's text result, so it can refer to the data when it composes its reply.
3. Builds the `Widget` payload — typed enough to feed the card, compact enough not to balloon context tokens.

The agent loop (`internal/agent/loop.go`) automatically picks up `Result.Widget` and:
- Fires a `widget` stream event so streaming clients see the card as soon as the tool returns
- Appends a `widget` `MessagePart` so the archived message has the card on re-render

Tools don't need to import realtime / event code — setting the Widget field is enough.

---

## tool calls, thinking, citations

### tool calls — `ToolCallExpandable` (thinking-block.tsx)

- Collapsed default. Click to show args + result.
- Status: running (pulsing dot), complete (check), error (X).
- Labels are past-tense after completion: "Searched 'pizza'", "Read news.ycombinator.com".
- When the result is web-search JSON, source pills render inline with favicons.

### thinking — `ThinkingBlock` (thinking-block.tsx)

- Auto-opens during streaming, auto-closes after completion.
- Shimmer text while streaming; "Thought for N seconds" once done.
- Scrolls with the latest tokens while streaming.

### citations

Two layers:

**Inline chips** — `transformCitations(text, sources)` in `inline-citations.tsx` turns bracketed references like `[1]` or `[1,3]` in the model's prose into superscript chips. Click: scrolls to the matching source pill and flashes it. Shift-click or middle-click: opens the URL.

**Source panel** — `CitationPanel` (thinking-block.tsx) or the per-source `SourcePillWithAnchor` at the end of the message. The panel is where the inline chips scroll to; each source carries `id="citation-<N>"` so the chip's `scrollIntoView` finds it.

### keyboard notes

None specific to the chat renderer today. Shift-click for new-tab on citations is the only keyboard affordance.

---

## streaming caret

A blinking block cursor appended after the streaming text. Scoped to a `.qorven-streaming` parent so it vanishes as soon as the message finalises. Pauses on hover to not interfere with text selection. CSS lives in `web/css/styles.css`.

---

## theming

All cards use CSS custom properties declared on the root:

- `hsl(var(--primary))` — accent color (agent brand)
- `hsl(var(--muted))` / `hsl(var(--muted-foreground))` — subtle surfaces and secondary text
- `hsl(var(--border))` / `hsl(var(--card))` — card surfaces
- `hsl(var(--destructive))` — errors
- Utility classes: `text-2xs` (extra small, 11px), `prose prose-sm prose-invert` for markdown, Tailwind's standard `rounded-xl`/`border`/`bg-*` tokens.

Changing the theme in Settings → Appearance rewrites these vars; all cards re-render automatically. Don't hardcode hex colours in cards.

---

## troubleshooting

| Symptom | Likely cause |
|---|---|
| Card never appears | Check `part-renderer.tsx` — is the case statement there? Did the tool set `Result.Widget`? |
| Card appears but blank | Widget `Data` shape doesn't match the card's expected props. Console-log the payload. |
| Inline citations don't scroll | Sources panel not rendered on the page yet. The chip falls back to opening the URL when the target id isn't found — check the source list is passed to both the chip transformer and `SourcePillWithAnchor`. |
| Streaming caret shows on archived messages | A missing `.qorven-streaming` class on the parent, or the caret was duplicated into the final message render. |
| Theme changes don't take effect on a card | Hardcoded colour somewhere in the card. Replace with an `hsl(var(--...))` class. |
