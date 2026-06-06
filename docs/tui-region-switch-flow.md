# TUI region switch flow

This documents the intended and current behavior of the `g` region switch flow.

## Current flow

1. The user presses `g`.
2. SeSaMe checks whether region switching is allowed:
   - active tunnels: region switching is blocked so background `aws` processes keep the same visible context.
   - missing provider factory: region switching is unavailable.
3. SeSaMe opens a region modal with manual input prefilled from the current region.
4. If a dynamic region provider is available, SeSaMe lazily loads available regions with `ec2:DescribeRegions` for the current auth/profile/region.
5. The user picks a loaded region with `up`/`down`, or edits the region name manually, and presses `Enter`.
6. Empty region input is rejected with `region is required`.
7. The same region closes the modal and reports `region unchanged: <region>`.
8. A different region:
   - closes the modal,
   - clears the current error,
   - enters `Loading instances...`,
   - rebuilds AWS SDK clients for the current profile/auth mode and requested region,
   - calls STS through the normal inventory load path,
   - refreshes inventory on success.

## Error states

If regions cannot be loaded dynamically, the modal remains usable and manual input still works. The health view also reports the region loading state or error.

Recognized region-loading failures are simplified for the TUI:

```text
ec2:DescribeRegions denied; type a region manually or ask for permission
```

Credential failures are shown as:

```text
AWS credentials unavailable; fix credentials or type a region manually
```

Other region-loading errors are shown after light cleanup and end with a manual-input hint.

If the requested region cannot be loaded, STS fails, credentials are expired, or inventory cannot be listed, the TUI stays usable and renders an error screen.

## Nuances

- `g` should work from the normal inventory view and from an error view.
- Region switching keeps the current auth mode and profile. Profile switching is a separate flow.
- Active tunnels block region switching because those processes were started with the old auth/region context.
- Dynamic region suggestions are optional. Manual input is always available.
- If clients are rebuilt successfully but inventory loading fails, the header should reflect the newly selected region while the body shows the error.
- Region suggestions are cached per auth mode, profile and region.

## Picker behavior

- Show the loaded region list when `ec2:DescribeRegions` succeeds.
- Mark the current region.
- Prefer keyboard selection, but keep manual input as an escape hatch for regions not present in the loaded list.
- If region loading fails or is unavailable, keep the current region available and allow manual input.
- Do not mutate AWS config or credentials files.
