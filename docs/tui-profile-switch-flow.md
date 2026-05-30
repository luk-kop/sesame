# TUI profile switch flow

This documents the intended and current behavior of the `p` profile switch flow.

## Current flow

1. The user presses `p`.
2. SeSaMe checks whether profile switching is allowed:
   - `env-active`: profile switching is ignored because environment credentials have precedence.
   - active tunnels: profile switching is blocked so background `aws` processes keep the same visible context.
   - missing provider factory: profile switching is unavailable.
3. In `profile-active`, SeSaMe opens a profile modal with discovered profiles and a text input prefilled with the current profile name.
4. The user picks a profile with `↑`/`↓`, or edits the profile name manually, and presses `Enter`.
5. Empty profile input is rejected with `profile name is required`.
6. The same profile name closes the modal and reports `profile unchanged: <profile>`.
7. A different profile name:
   - closes the modal,
   - clears the current error,
   - enters `Loading instances...`,
   - rebuilds AWS SDK clients for the requested profile and current region,
   - calls STS through the normal inventory load path,
   - refreshes inventory on success.

## Error states

If the requested profile cannot be loaded, STS fails, credentials are expired, or inventory cannot be listed, the TUI stays usable and renders an error screen.

Recognized IMDS/credentials failures are shown as:

```text
Error: AWS credentials unavailable for profile <profile>. No EC2 IMDS role found.
       Check credentials or press p to choose another profile.
```

Other AWS SDK errors are currently displayed after light cleanup of noisy service prefixes.

## Nuances

- `p` should work from the normal inventory view and from an error view.
- `p` is intentionally disabled in `env-active`; selected profiles must not override explicit environment credentials.
- The profile switch keeps the current region. Region switching is a separate flow.
- Active tunnels block profile switching because those processes were started with the old auth/region context.
- If clients are rebuilt successfully but inventory loading fails, the header should reflect the newly selected profile while the body shows the error.
- The current implementation lists discovered profiles and keeps manual text input as an escape hatch.
- The profile picker should read from the same shared config sources that AWS CLI/SDK use:
  - `AWS_CONFIG_FILE` if set, otherwise `~/.aws/config`,
  - `AWS_SHARED_CREDENTIALS_FILE` if set, otherwise `~/.aws/credentials`.
- In the config file, named profile sections are written as `[profile name]`; in the credentials file they are written as `[name]`. The `default` profile is `[default]` in both files.

## Picker behavior

- Show the union of profile names from shared config and shared credentials files.
- Mark the current profile.
- Prefer keyboard selection, but keep manual input as an escape hatch for profiles not present in readable files.
- If no profiles can be discovered, keep `default` available and allow manual input.
- Do not read or display secret values.
- Do not mutate AWS config or credentials files.
