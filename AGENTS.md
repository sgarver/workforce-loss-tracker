# Agent Instructions for Project Changes

## Process for Making Changes to Go Files

Whenever changes are made to Go files in this project, follow this process:

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
- [ ] **Todo Updated**: `todo.md` updated if task completed.

## Explicit Approval Protocol

To ensure strict compliance with user approval requirements:

**1. Pre-Commit Confirmation**
Before any commit, always include this prompt:
```
**Ready to commit?** Please reply with "commit" to approve, or provide feedback for changes.
```

**2. Checklist Accuracy**
- Only check [x] items that were actually completed
- Mark N/A items as such (e.g., "Build Success: N/A - documentation only")
- Never check "User Approval" until user explicitly approves

**3. Protocol Reminder**
Include this reminder for all changes:
*"Following AGENTS.md protocol: Waiting for explicit user approval before committing"*

**4. No Assumptions**
Never assume approval for any change type - always require explicit "commit" confirmation, even for documentation, README updates, or minor fixes.

## Task Management

- All current and ongoing tasks are stored in `todo.md`.
- Update `todo.md` with new tasks, mark completed tasks, and update timestamps as needed.

## Final Step

- After completing changes, update the list time in `todo.md` to reflect the current timestamp.