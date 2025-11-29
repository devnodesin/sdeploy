## Overview

This PRD covers configuration enhancements, git behavior improvements, logging enhancements, and variable renaming for SDeploy v1.0. These changes improve clarity, add missing git clone/pull logic, enhance logging, and align with the updated SPEC.md.

**Test Repository:** `https://github.com/devnodesin/sdeploy-test.git`

## Requirements

### 1. Version and Service Status Logging

- Add a constant `Version = "v1.0"` and `ServiceName = "SDeploy"` in `main.go`
- On daemon startup, log: `"SDeploy v1.0 - Service started"`
- On daemon shutdown, log: `"SDeploy v1.0 - Service terminated"`
- Include version in `printUsage()` output

### 2. Rename `git_path` to `local_path`

- In `config.go`, rename `GitPath` field to `LocalPath` and JSON tag from `"git_path"` to `"local_path"`
- Update all references in `deploy.go`, `webhook.go`, and any other files
- Update `samples/config.json` to use `local_path`
- Update `SPEC.md` to document `local_path` instead of `git_path`

### 3. Email Configuration Validation and Disable Logic

- In `main.go` or `email.go`, add function `IsEmailConfigValid(cfg *EmailConfig) bool`:
  - Returns `false` if `cfg` is `nil`
  - Returns `false` if any required field is empty: `smtp_host`, `smtp_user`, `smtp_pass`, `email_sender`
  - Returns `false` if `smtp_port` is `0`
  - Returns `true` otherwise
- On startup, if email config is invalid:
  - Log: `"Email notification disabled: email_config is missing or invalid."`
  - Do NOT create the `EmailNotifier`
- Per-project: If `email_recipients` is absent or empty, skip sending email for that project (already partially implemented, ensure log message)

### 4. Git Branch Default to `main`

- In `LoadConfig()` or `validateConfig()`, after loading config:
  - For each project, if `GitBranch` is empty, set it to `"main"`
- This ensures all branch comparisons use the default branch

### 5. Git Repository and Clone/Pull Logic

- In `deploy.go`, update the `Deploy()` function to handle git operations:

**Git Logic (before execute_command):**

```
if git_repo is empty or absent:
    - Log: "No git_repo configured, treating local_path as local directory"
    - Skip all git operations (no clone, no pull)
    - Proceed directly to execute_command
else:
    - Check if local_path exists and is a git repo (has .git directory)
    - if local_path does not exist OR is not a git repo:
        - Clone: git clone <git_repo> <local_path>
        - Checkout branch: git checkout <git_branch>
        - Log: "Cloned repository to local_path"
    - else (repo already cloned):
        - Log: "Repository already cloned at local_path"
        - if git_update is true:
            - git pull
            - Log: "Executed git pull"
        - else:
            - Log: "git_update is false, skipping git pull"
```

- Add helper functions:
  - `isGitRepo(path string) bool` - checks if `.git` directory exists
  - `gitClone(ctx context.Context, repoURL, localPath, branch string) error`
  - Rename existing `gitPull()` for clarity

### 6. Startup Logging of All Settings and Configs

- On daemon startup, after loading config, log:
  - Global settings: `listen_port`, `log_filepath`, email config status
  - For each project, log all non-sensitive fields:
    - `name`, `webhook_path`, `local_path`, `execute_path`, `git_repo` (if set), `git_branch`, `git_update`, `timeout_seconds`, `email_recipients` count
  - Do NOT log secrets (`webhook_secret`, `smtp_pass`)

Example format:
```
[INFO] [SDeploy] Configuration loaded:
[INFO] [SDeploy]   Listen Port: 8080
[INFO] [SDeploy]   Log File: /var/log/sdeploy/daemon.log
[INFO] [SDeploy]   Email Notifications: enabled
[INFO] [SDeploy] Project [1]: SDeploy Test Project
[INFO] [SDeploy]   - Webhook Path: /hooks/sdeploy-test
[INFO] [SDeploy]   - Local Path: /var/repo/sdeploy-test
[INFO] [SDeploy]   - Git Repo: https://github.com/devnodesin/sdeploy-test.git
[INFO] [SDeploy]   - Git Branch: main
[INFO] [SDeploy]   - Git Update: true
[INFO] [SDeploy]   - Execute Path: /var/repo/sdeploy-test
[INFO] [SDeploy]   - Execute Command: sh build.sh
[INFO] [SDeploy]   - Timeout: 300s
[INFO] [SDeploy]   - Email Recipients: 1
```

### 7. Per-Build Config Logging

- In `deploy.go`, at the start of `Deploy()` (after acquiring lock), log the project configuration:
  - Log project name, local_path, git_repo (if any), git_branch, git_update, execute_path, execute_command
  - Format: `"Build config: name=X, local_path=Y, git_repo=Z, ..."`

### 8. Update Sample Config

- Update `samples/config.json`:
  - Rename `git_path` to `local_path`
  - Use test repo URL: `https://github.com/devnodesin/sdeploy-test.git`

## Acceptance

### Version and Service Status
- [ ] `Version` constant is `"v1.0"`
- [ ] Startup log shows: `"SDeploy v1.0 - Service started"`
- [ ] Shutdown log shows: `"SDeploy v1.0 - Service terminated"`
- [ ] `sdeploy -h` shows version in output

### Rename git_path to local_path
- [ ] Config struct uses `LocalPath` with JSON tag `"local_path"`
- [ ] All code references updated (`deploy.go`, etc.)
- [ ] `samples/config.json` uses `local_path`
- [ ] SPEC.md updated to document `local_path`

### Email Validation
- [ ] `IsEmailConfigValid()` function exists and validates all required fields
- [ ] If email config invalid, log: `"Email notification disabled: email_config is missing or invalid."`
- [ ] If email config invalid, `EmailNotifier` is NOT created
- [ ] Per-project: No email sent if `email_recipients` is empty

### Git Branch Default
- [ ] If `git_branch` is empty in config, it defaults to `"main"`
- [ ] Branch comparison uses default value correctly

### Git Clone/Pull Logic
- [ ] If `git_repo` is empty: No git operations, local_path treated as local directory
- [ ] If `git_repo` is set and repo not cloned: Clone repository
- [ ] If `git_repo` is set and repo already cloned: Skip clone
- [ ] If `git_update` is true: Run git pull after clone check
- [ ] If `git_update` is false (default): Skip git pull
- [ ] All git operations logged appropriately

### Startup Logging
- [ ] All global settings logged on startup
- [ ] All project configs logged on startup (non-sensitive fields only)
- [ ] Secrets are NOT logged

### Per-Build Logging
- [ ] Each deployment logs the project config at build start

### Tests
- [ ] All existing tests pass
- [ ] New tests for `IsEmailConfigValid()`
- [ ] New tests for `isGitRepo()`
- [ ] New tests for git clone logic
- [ ] New tests for default branch logic
- [ ] Run `go test ./cmd/sdeploy/...` passes
