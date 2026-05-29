# SeSaMe

A TUI for working with EC2 instances through AWS Systems Manager Session Manager.

[![Go](https://img.shields.io/badge/Go-1.26-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![Built with Bubble Tea](https://img.shields.io/badge/built%20with-Bubble%20Tea-ff75b7)](https://github.com/charmbracelet/bubbletea)
[![AWS SDK for Go v2](https://img.shields.io/badge/AWS%20SDK-Go%20v2-FF9900?logo=amazonaws&logoColor=white)](https://github.com/aws/aws-sdk-go-v2)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

> [!NOTE]
> The name is stylized as **SeSaMe** — the capital `S`, `S`, `M` highlight AWS **SSM**. The whole word is a nod to *"open sesame"*, the magic phrase from *Ali Baba and the Forty Thieves* that opens the sealed cave: say the words and the door swings open. SeSaMe does the same for EC2 instances reachable through SSM — one keystroke and you're inside, no bastion host or SSH key to dig up.

## Overview

SeSaMe is a terminal UI and companion CLI for everyday work with EC2 instances that are managed through AWS Systems Manager. Instead of juggling long `aws ssm start-session` invocations, you browse a live inventory, see which instances actually have a healthy SSM agent, and open a shell or port-forwarding tunnel with a single keystroke.

Highlights:

- **Inventory view** — lists EC2 instances correlated with `ssm:DescribeInstanceInformation`, so the SSM agent status (`online`, `connection-lost`, `not-managed`, `unknown`) is visible up front.
- **Interactive search & filters** — local, case-insensitive search across name, ID, IPs, tags, EC2 state and SSM status; explicit filters for region, state and SSM status.
- **Shell sessions** — `Enter` (or `sesame shell <instance-id>`) hands the terminal over to `aws ssm start-session` and returns to the TUI when the session ends.
- **Port forwarding** — open local/remote port tunnels from a modal, manage their lifecycle in a dedicated view, and get prompted before quitting with active tunnels.
- **Auth-aware** — distinguishes `env-active` (credentials from environment variables) from `profile-active` (AWS profile), keeps SDK and AWS CLI on the same identity, and always resolves the region explicitly.
- **CLI for scripts** — `sesame list` with stable `--output json` (including `auth`, `region`, `account`, `arn` and `warnings`), plus consistent exit codes for usage, preflight, runtime and missing-dependency errors.

## Build

### Make Targets

### Release Artifacts

## Usage

