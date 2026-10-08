# Console and customer portal theme

The console theme adapts the publicly observable design language of
[NexKr](https://nexkr.sh/) (reviewed 2026-10-08): soft layered surfaces,
blue-violet emphasis, soft panel corners and short eased transitions.
The light palette uses the original console's near-white, cool backgrounds
and a white sidebar status card instead of broad gray surfaces.
It does not redistribute the site's stylesheet, fonts, scripts or artwork,
and it has no runtime dependency on the reference website.

## Implementation

- `src/lib/adminConsoleTheme.ts` owns light/dark palettes and Ant Design tokens.
- `src/components/AdminConsoleTheme.tsx` scopes tokens and portal styling to the
  internal console and `/customer`. `/site`, `/official` and `?page=site` retain
  their existing theme. Subpath deployments use the existing `appPathname()`.
- `src/styles/console.css` is the shared local visual layer. It does not
  change table widths, horizontal scrolling, fixed columns or save actions.
- `src/styles/customer-console.css` adapts customer cards, topology, support
  and mobile layout. Customer login opts into the shared split login with
  customer-specific copy. Existing account custom CSS and API behavior are
  retained. QR codes use a white background in both themes for scanning.
- `prefers-reduced-motion` disables entrance animation and component motion.
- Financial success, warnings and destructive actions keep semantic colors
  separate from the brand accent. Primary/danger button text passes 4.5:1
  contrast in both palette variants.

## Verification

Run `npm run test:console-theme` and `npm run build` from `web/`.
For visual review, use a disposable local server database and demo snapshots,
not production credentials or customer data. Review desktop/light/dark,
announcement modals, customer login/links, 390px mobile navigation and
public-route isolation. The regular local Vite preview can proxy the live API;
isolated visual fixtures must use a separate test server, never mutate live data.
Existing frontend regression scripts cover navigation, finance, expiry,
network summaries, search and routing deletion.

Versioning and publication follow the project's normal release workflow.
