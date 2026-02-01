# Agent Instructions for Project Changes

## Process for Making Changes to Go Files

Whenever changes are made to Go files in this project, follow this process:

0. **Pre-Push Checks** (Optional but Recommended):
    - Run `./pre-push-check.sh` to validate code quality locally
    - Includes build, test, and YAML linting checks
    - Prevents pushing broken code

1. **Verify Build Success**:
   - Run `go build -o layoff-tracker .` to ensure the project compiles successfully.
   - Check for any compilation errors and fix them.

2. **Run Unit Tests**:
   - Execute `go test ./...` to run all unit tests.
   - Ensure all tests pass before proceeding.

3. **Restart the Site**:
   - Stop any running instances: `pkill -f layoff-tracker`
   - Build and start the server: `./layoff-tracker &`
   - Confirm the server is running: Wait for `curl -s http://127.0.0.1:8080/ping` to return "pong"
   - Use curl to verify that the changes fix the specific issue (e.g., curl relevant endpoints or pages)

4. **Manual Verification**:
   - **MANDATORY**: Prompt the user to perform manual verification of the changes.
   - **DO NOT PROCEED WITHOUT EXPLICIT USER APPROVAL**. Always wait for the user to explicitly say "commit" (or similar confirmation like "approved") before advancing to step 5. Do not infer approval from phrases like "please proceed" or "looks good"—require direct confirmation.

5. **Commit to Git**:
   - **ONLY AFTER USER SAYS "COMMIT" FOR ANY CHANGE**: Do not commit any changes (Go or otherwise) until the user explicitly says "commit" (or equivalent). This prevents premature commits.
   - Commit the changes: `git add .` and `git commit -m "Description of changes"`
   - Update the completed task in `todo.md` and update the timestamp.
   - Ask the user about the next task.

## Post-Change Checklist (For AI Responses)
After making changes, include this checklist in responses to ensure protocol adherence. Only mark items as [x] if they are truly completed—leave pending items as [ ] and note their status clearly to avoid confusion.

- [ ] **Build Success**: `go build -o layoff-tracker .` - No errors.
- [ ] **Run Tests**: `go test ./...` - All pass.
- [ ] **Restart Site**: Server stopped, restarted, `curl -s http://127.0.0.1:8080/ping` returns "pong".
- [ ] **Curl Verification**: Specific curls to verify changes (e.g., endpoints, UI elements).
- [ ] **Manual Verification Prompt**: Asked user to verify before committing.
- [ ] **User Approval**: Confirmed user explicitly said "commit" (or equivalent) before commit.
- [ ] **Commit Done**: Changes committed with descriptive message.
- [ ] **Issue Status Updated**: Related GitHub issues updated or closed.
- [ ] **Issues Closed**: Related GitHub issues closed or moved to Done.

## Explicit Approval Protocol

To ensure strict compliance with user approval requirements:

**1. Pre-Commit Confirmation**
Before any commit, always include this prompt:
```
**Ready to commit?** Please reply with "commit" to approve, or provide feedback for changes.
```

**2. Pre-Merge Confirmation**
Before merging dev → `staging` or `staging` → `main`, always include this prompt:
```
**Ready to promote?** Please reply with "approve" to proceed, or provide feedback for changes.
```

**3. Approval Order is Critical**
- **NEVER commit before approval** - Wait for explicit "commit" confirmation
- **NEVER merge between branches before approval** - Wait for explicit "approve" confirmation
- Show checklist and prompt FIRST
- Commit or merge SECOND (only after approval received)
- If user provides feedback instead of the approval keyword, address feedback and re-prompt

**4. Checklist Accuracy**
- Only check [x] items that were actually completed
- Mark N/A items as such (e.g., "Build Success: N/A - documentation only")
- Never check "User Approval" until user explicitly approves

**5. Protocol Reminder**
Include this reminder for all changes:
*"Following AGENTS.md protocol: Waiting for explicit user approval before committing"*

**6. No Assumptions**
Never assume approval for any change type - always require explicit "commit" confirmation, even for documentation, README updates, or minor fixes.

## Task Management

- All current and ongoing tasks are tracked in the GitHub Project:
  https://github.com/users/sgarver/projects/1
- Use GitHub Issues as the source of truth for roadmap items.

## Branching Policy (Milestone Work)

When starting a new milestone:
- Create an ephemeral dev branch from `staging`.
- Only work on that dev branch until the milestone is completed and verified locally.
- After user approval, merge dev → `staging`, then `staging` → `main`, then deploy.

Recommended dev branch naming: `feature/<milestone>`.

## Complete SDLC Process

For all changes (not just Go files), follow this Software Development Lifecycle:

### 1. Local Development
- Make code changes locally
- Run `./pre-push-check.sh` for local validation (build, tests, linting)
- Follow the "Process for Making Changes to Go Files" above if applicable
- Test locally: build, unit tests, integration tests, manual verification

### 2. Staging Deployment
- **Merge dev → `staging`** only after user approval (use approval keyword below)
- Push to `staging` for CI validation
- CI runs: Tests, Security scan, Build, Integration tests
- Monitor staging CI results
- If staging fails, fix issues and re-push to staging
- PR bodies should include `Closes #<issue>` for related work

### 3. Production Deployment
- **Only after staging passes**: Push/merge to `main` branch
- CI runs again on main
- Manual production deployment via GitHub Actions (admin only)
- Verify production functionality
- Close or mark related issues as Done in the project board
- Tag the release with the milestone version (e.g., `v0.8.0`) and publish release notes

### 4. Rollback (If Needed)
- If production issues arise, use documented rollback steps in `DEPLOY.md`
- Restore from timestamped backup on server

### Key Principles
- **Never push directly to main** - always validate on staging first
- **Do not commit directly to `main` for milestone work**; use dev branch then merge
- **Require explicit user approval** before any commits
- **Test thoroughly** at each stage
- **Document all processes** in `DEPLOY.md`

## Final Step

- After completing changes, update the list time in `todo.md` to reflect the current timestamp.
