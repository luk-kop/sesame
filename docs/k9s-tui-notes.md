# K9s TUI notes

This documents useful K9s interaction patterns and how they map to SeSaMe.

## Reference behavior

K9s is a full-screen terminal UI centered around resource table views. The main screen is a scrollable table, with contextual header information at the top and key hints at the bottom.

Relevant K9s patterns:

- `/` opens a filter mode for the current resource view.
- `:` opens command mode for switching resource views or scopes.
- `?` shows contextual key bindings.
- `<esc>` exits command or filter mode.
- Resource actions are bound to short keys, such as describe, view, edit, logs, or shell.
- Views support a narrow/default mode and a wide mode with additional columns.
- Resource table columns can be customized per resource.
- Filtering supports regular expressions, inverse filters, label selectors, and fuzzy filtering.
- K9s continuously watches and refreshes cluster resources.

Official references:

- <https://k9scli.io/>
- <https://k9scli.io/topics/commands/>
- <https://k9scli.io/topics/columns/>

## SeSaMe implications

SeSaMe should follow the same high-level shape where it fits the EC2/SSM domain:

- Keep EC2 instances as the primary scrollable table view.
- Keep details and actions focused on the selected instance.
- Keep the header for runtime context: auth mode, profile, region, account, status, filter state and active tunnels.
- Keep the footer for contextual key hints.
- Prefer keyboard-first workflows.
- Avoid rendering the entire inventory when only a terminal-sized viewport is visible.

The current viewported instance rendering is aligned with this direction. It prevents large AWS environments from making the TUI render thousands of rows on every frame.

## Implemented first slice

1. Viewported instance rendering.

   The table renders only the rows visible in the current terminal-sized window, and the table title exposes both the selected range and total result count:

   ```text
   Instances (9-24 of 1300)
   ```

2. Responsive details.

   On wide terminals, details stay beside the table. On narrow terminals, details remain a separate focused view toggled with `d` or `Tab`.

3. Wide mode toggle.

   The default table keeps the operational columns that are most useful for SSM workflows:

   ```text
   NAME  INSTANCE ID  STATE  SSM  PRIVATE IP
   ```

   `w` toggles extra columns when the terminal is wide enough:

   ```text
   NAME  INSTANCE ID  TYPE  STATE  SSM  PRIVATE IP  PUBLIC IP  REGION
   ```

   Very narrow terminals still fall back to fewer columns to keep the table readable.

4. Explicit filter bar.

   `/` renders a visible filter bar and filters the local inventory as the user types. `Esc` and `Enter` close the filter bar while keeping the filter active; `Ctrl+U` clears it. The closed header still shows the active filter.

   The first slice keeps filtering as simple case-insensitive substring matching across name, instance ID, IPs, tags, EC2 state, SSM status and region.

5. Sorting shortcuts.

   Sorting is local to the loaded inventory. The first shortcut set uses uppercase keys to avoid conflicts with existing actions:

   - `N`: name
   - `I`: instance ID
   - `S`: state
   - `M`: SSM status
   - `P`: private IP

   Repeating the active sort key reverses the direction, and the header shows the active sort.

## Remaining follow-ups

- Add richer filter modes only when the UX is clear: regular expressions, inverse filters, label/tag selectors, or fuzzy filtering.
- Consider table column customization per user or per resource view.
- Consider continuous refresh/watch behavior, with care around AWS API cost, pagination, throttling and selection stability.

## Non-goals

- Do not copy Kubernetes-specific resource navigation such as `:pod` or `:svc`.
- Do not add mutation-heavy actions by default. SeSaMe should stay centered on SSM session workflows.
- Do not make CLI `sesame list` behave like a TUI. The CLI should remain script-friendly, while the TUI follows K9s-style interaction patterns.
