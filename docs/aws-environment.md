# AWS environment variables

SeSaMe follows the same high-level AWS configuration conventions as the AWS CLI and AWS SDK.

## Credentials and auth mode

If both of these variables are set, SeSaMe uses `env-active` mode:

- `AWS_ACCESS_KEY_ID`
- `AWS_SECRET_ACCESS_KEY`

Optional session credentials are also passed through the normal AWS SDK/CLI chain:

- `AWS_SESSION_TOKEN`

In `env-active` mode, profile selection is ignored. Environment credentials are treated as an explicit user choice and take precedence over profiles.

If environment credentials are not active, SeSaMe uses `profile-active` mode. The profile comes from:

1. `--profile <name>`
2. `AWS_PROFILE`
3. `default`

## Region

SeSaMe resolves the region from:

1. `--region <region>`
2. `AWS_REGION`
3. `AWS_DEFAULT_REGION`
4. the active AWS profile/config loaded by the AWS SDK

The resolved region is used consistently by the SDK inventory calls and by child `aws ssm start-session` processes.

## Shared config files

The profile picker reads profile names from the same shared files used by AWS tooling:

- `AWS_CONFIG_FILE`, or `~/.aws/config` when unset
- `AWS_SHARED_CREDENTIALS_FILE`, or `~/.aws/credentials` when unset

Only profile names are read. SeSaMe does not display secret values and does not mutate these files.

In `~/.aws/config`, named profiles use `[profile name]`; in `~/.aws/credentials`, they use `[name]`. The `default` profile is `[default]` in both files.

## Runtime profile switch

Press `p` in the TUI to choose a profile when SeSaMe is in `profile-active` mode.

Profile switching is disabled when:

- `env-active` credentials are set,
- active tunnels are running,
- profile switching is unavailable in the current runtime context.

Press `g` in the TUI to choose a region. The picker opens immediately with manual input enabled, then lazily loads available regions with `ec2:DescribeRegions` for the current auth/profile/region. If regions cannot be loaded, SeSaMe shows the error in the modal and health view, and manual input remains available.

Region switching is disabled when:

- active tunnels are running,
- region switching is unavailable in the current runtime context.
