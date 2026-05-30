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
- **Interactive search & filters** — local, case-insensitive search across name, ID, IPs, tags, EC2 state and SSM status; explicit filters for region, state and SSM status.
- **Shell sessions** — `Enter` (or `sesame shell <instance-id>`) hands the terminal over to `aws ssm start-session` and returns to the TUI when the session ends.
- **Port forwarding** — open local/remote port tunnels from a modal, manage their lifecycle in a dedicated view, and get prompted before quitting with active tunnels.
- **Auth-aware** — distinguishes `env-active` (credentials from environment variables) from `profile-active` (AWS profile), keeps SDK and AWS CLI on the same identity, and always resolves the region explicitly.
- **CLI for scripts** — `sesame list` with stable `--output json` (including `auth`, `region`, `account`, `arn` and `warnings`), plus consistent exit codes for usage, preflight, runtime and missing-dependency errors.

See [AWS environment variables](docs/aws-environment.md) for supported auth, region and shared config file behavior.

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
| `build` | Build the `sesame` binary into `bin/`. |
| `run` | Run the CLI help. |
| `clean` | Remove build artifacts (`bin/` and the local build cache). |

### Release Artifacts

## Usage

Start the TUI:

```sh
sesame --profile dev --region eu-central-1
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
- `ssm:DescribeInstanceInformation` denied: inventory can still show EC2 instances, but SSM status becomes `unknown`/`error` with a warning.
- `sts:GetCallerIdentity` denied or invalid credentials: startup and CLI commands fail before inventory or sessions.
- `ssm:StartSession` denied: preflight can pass, but shell or tunnel startup fails in AWS CLI.
- `ssmmessages:OpenDataChannel` denied: the session starts but cannot attach the data channel correctly.
