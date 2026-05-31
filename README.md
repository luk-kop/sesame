# SeSaMe

A TUI for working with EC2 instances through AWS Systems Manager Session Manager.

[![Go](https://img.shields.io/badge/Go-1.26-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![Built with Bubble Tea](https://img.shields.io/badge/built%20with-Bubble%20Tea-ff75b7)](https://github.com/charmbracelet/bubbletea)
[![AWS SDK for Go v2](https://img.shields.io/badge/AWS%20SDK-Go%20v2-FF9900?logo=amazonaws&logoColor=white)](https://github.com/aws/aws-sdk-go-v2)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

> [!NOTE]
> The name is stylized as **SeSaMe** — the capital `S`, `S`, `M` highlight AWS **SSM**. The whole word is a nod to *"open sesame"*, the magic phrase from *Ali Baba and the Forty Thieves* that opens the sealed cave: say the words and the door swings open. SeSaMe does the same for EC2 instances reachable through SSM — one keystroke and you're inside, no bastion host or SSH key to dig up.

## Overview

**SeSaMe** is a terminal UI and companion CLI for everyday work with EC2 instances that are managed through **AWS Systems Manager**. Instead of juggling long `aws ssm start-session` invocations, you browse a live inventory, see which instances actually have a healthy SSM agent, and open a shell or port-forwarding tunnel with a single keystroke.

Highlights:

- **Inventory view** — lists EC2 instances correlated with `ssm:DescribeInstanceInformation`, so the SSM agent status (`online`, `connection-lost`, `not-managed`, `unknown`) is visible up front.
- **Interactive search & filters** — local, case-insensitive search across name, ID, IPs, tags, EC2 state and SSM status; CLI filters for name, state and SSM status; runtime region switching in the TUI.
- **Shell sessions** — `Enter` (or `sesame shell <instance-id>`) hands the terminal over to `aws ssm start-session` and returns to the TUI when the session ends.
- **Port forwarding** — open local/remote port tunnels from a modal, manage their lifecycle in a dedicated view, and get prompted before quitting with active tunnels.
- **Auth-aware** — distinguishes `env-active` (credentials from environment variables) from `profile-active` (AWS profile), keeps SDK and AWS CLI on the same identity, and always resolves the region explicitly.
- **CLI for scripts** — `sesame list` with stable `--output json` (including `auth`, `region`, `account`, `arn` and `warnings`), plus consistent exit codes for usage, preflight, runtime and missing-dependency errors.

See [AWS environment variables](docs/aws-environment.md) for supported auth, region and shared config file behavior.

## Requirements

Runtime:

- AWS credentials available through environment variables, an AWS profile, or another AWS SDK-supported provider.
- AWS region from `--region`, environment variables, profile config, or manual TUI selection.
- `aws` CLI in `PATH` for TUI shell/tunnel sessions and `sesame shell` / `sesame tunnel`.
- `session-manager-plugin` in `PATH` for SSM shell/tunnel sessions.

Build:

- Go 1.26.x.
- `make`.
- `golangci-lint` for local linting and pre-commit checks.

AWS permissions:

- Inventory permissions listed below.
- Session permissions for shell/tunnel usage.
- Optional `ec2:DescribeRegions` for dynamic region suggestions in the TUI.

## Install

### Install From GitHub Releases

This is the recommended path for workstation use. Download the latest Linux archive
for your CPU architecture from [GitHub Releases](https://github.com/luk-kop/sesame/releases).

For x86_64 / amd64:

```sh
curl -L -o sesame_linux_amd64.tar.gz \
  https://github.com/luk-kop/sesame/releases/latest/download/sesame_linux_amd64.tar.gz

tar -xzf sesame_linux_amd64.tar.gz
mkdir -p ~/.local/bin
install -m 0755 sesame_*/sesame ~/.local/bin/sesame
```

For arm64:

```sh
curl -L -o sesame_linux_arm64.tar.gz \
  https://github.com/luk-kop/sesame/releases/latest/download/sesame_linux_arm64.tar.gz

tar -xzf sesame_linux_arm64.tar.gz
mkdir -p ~/.local/bin
install -m 0755 sesame_*/sesame ~/.local/bin/sesame
```

Make sure `~/.local/bin` is in `PATH`:

```sh
echo "$PATH" | tr ':' '\n' | grep -x "$HOME/.local/bin"
```

If it is missing, add this to your shell profile:

```sh
export PATH="$HOME/.local/bin:$PATH"
```

Verify the installation:

```sh
sesame --version
```

### Build From Source

Use this when developing SeSaMe or when a release binary is not available for your
platform:

```sh
git clone https://github.com/luk-kop/sesame.git
cd sesame
make build
mkdir -p ~/.local/bin
install -m 0755 bin/sesame ~/.local/bin/sesame
sesame --version
```

## Build

### Make Targets

Run `make help` to list the available targets:

| Target | Description |
| --- | --- |
| `tidy` | Sync Go module dependencies (`go mod tidy`). |
| `update-patch` | Update dependencies to the latest patch releases (safe), then tidy. |
| `update-minor` | Update dependencies to the latest minor + patch releases, then tidy. |
| `fmt` | Format Go sources with `gofmt`. |
| `test` | Run all tests. |
| `build` | Build the `sesame` binary with version metadata into `bin/`. |
| `build-release` | Build Linux release archives and checksums into `bin/`. |
| `run` | Run the CLI help. |
| `clean` | Remove build artifacts (`bin/` and the local build cache). |

## Usage

Start the TUI:

```sh
sesame --profile dev --region eu-central-1
```

Print build metadata:

```sh
sesame --version
```

List instances from the active region:

```sh
sesame list --profile dev --region eu-central-1
sesame list --name api --ssm online --output json
```

Open an interactive shell session:

```sh
sesame shell i-0123456789abcdef0 --profile dev --region eu-central-1
```

Open a foreground port-forwarding session:

```sh
sesame tunnel i-0123456789abcdef0 --local-port 15432 --remote-port 5432 --profile dev --region eu-central-1
```

`sesame list` uses the AWS SDK only. The TUI, `shell`, and `tunnel` require both `aws` and `session-manager-plugin` in `PATH` because sessions are started through `aws ssm start-session`.

In the TUI, `g` opens the region picker. The input is always editable, and SeSaMe lazily loads available regions with `ec2:DescribeRegions` for the current profile/region. If region loading fails, manual input still works.

## Exit Codes

| Code | Meaning |
| --- | --- |
| `0` | Success. |
| `1` | Runtime error, such as AWS API, credentials, or region resolution failure. |
| `2` | CLI usage error, such as a missing argument, invalid port, or unsupported output/filter value. |
| `3` | Session preflight failed, such as a missing instance or an instance that is not SSM `online`. |
| `4` | Local session dependency is missing, such as `aws` or `session-manager-plugin`. |

## IAM Permissions

SeSaMe needs separate permissions for inventory and sessions. Inventory is read-only and is used by the TUI and `sesame list`. Shell and tunnel commands need Session Manager permissions in addition to inventory preflight checks.

Minimal inventory permissions:

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "SesameInventory",
      "Effect": "Allow",
      "Action": [
        "ec2:DescribeInstances",
        "ssm:DescribeInstanceInformation"
      ],
      "Resource": "*"
    },
    {
      "Sid": "SesameIdentity",
      "Effect": "Allow",
      "Action": "sts:GetCallerIdentity",
      "Resource": "*"
    }
  ]
}
```

Dynamic region suggestions in the TUI require one extra permission. This is not required for `sesame list`, inventory loading, shell sessions, or tunnels when a region is already provided by `--region`, env vars, profile config, or manual TUI input.

TUI region suggestions permission:

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "SesameRegionPicker",
      "Effect": "Allow",
      "Action": "ec2:DescribeRegions",
      "Resource": "*"
    }
  ]
}
```

Add these permissions for shell sessions and port forwarding:

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "SesameStartSessions",
      "Effect": "Allow",
      "Action": [
        "ssm:StartSession"
      ],
      "Resource": [
        "arn:aws:ec2:*:*:instance/*",
        "arn:aws:ssm:*::document/SSM-SessionManagerRunShell",
        "arn:aws:ssm:*::document/AWS-StartPortForwardingSession"
      ]
    },
    {
      "Sid": "SesameSessionChannels",
      "Effect": "Allow",
      "Action": [
        "ssmmessages:CreateControlChannel",
        "ssmmessages:CreateDataChannel",
        "ssmmessages:OpenControlChannel",
        "ssmmessages:OpenDataChannel",
        "ssm:TerminateSession",
        "ssm:ResumeSession"
      ],
      "Resource": "*"
    }
  ]
}
```

For a stricter policy, replace the wildcard instance resource with specific account, region, instance, or tag-scoped conditions used by your AWS organization. If you later enable remote-host forwarding, also allow `arn:aws:ssm:*::document/AWS-StartPortForwardingSessionToRemoteHost`.

Common `AccessDenied` causes:

- `ec2:DescribeInstances` denied: inventory cannot load and `list` fails.
- `ec2:DescribeRegions` denied: dynamic region suggestions cannot load, but manual region input still works.
- `ssm:DescribeInstanceInformation` denied: inventory can still show EC2 instances, but SSM status becomes `unknown`/`error` with a warning.
- `sts:GetCallerIdentity` denied or invalid credentials: startup and CLI commands fail before inventory or sessions.
- `ssm:StartSession` denied: preflight can pass, but shell or tunnel startup fails in AWS CLI.
- `ssmmessages:OpenDataChannel` denied: the session starts but cannot attach the data channel correctly.
