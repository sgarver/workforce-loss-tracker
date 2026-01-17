# Agent Instructions for Project Changes

## Process for Making Changes to Go Files

**CRITICAL: Never commit changes without explicit user approval after manual verification. Always prompt the user to verify changes before proceeding to step 5.**

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
   - Confirm the server is running: `curl -s http://127.0.0.1:8080/ping` should return "pong"
   - Use curl to verify that the changes fix the specific issue (e.g., curl relevant endpoints or pages)

4. **Manual Verification**:
   - **MANDATORY**: Prompt the user to perform manual verification of the changes.
   - **DO NOT PROCEED WITHOUT EXPLICIT USER APPROVAL**. Always wait for the user to explicitly say "commit" (or similar confirmation like "approved") before advancing to step 5. Do not infer approval from phrases like "please proceed" or "looks good"—require direct confirmation.

5. **Commit to Git**:
   - **ONLY AFTER USER SAYS "COMMIT"**: Do not commit until the user explicitly says "commit" (or equivalent). This prevents premature commits.
   - Commit the changes: `git add .` and `git commit -m "Description of changes"`
   - Update the completed task in `todo.md` and update the timestamp.
   - Ask the user about the next task.

## Post-Change Checklist (For AI Responses)
After making changes, include this checklist in responses to ensure protocol adherence:

- [ ] **Build Success**: `go build -o layoff-tracker .` - No errors.
- [ ] **Run Tests**: `go test ./...` - All pass.
- [ ] **Restart Site**: Server stopped, restarted, `curl -s http://127.0.0.1:8080/ping` returns "pong".
- [ ] **Curl Verification**: Specific curls to verify changes (e.g., endpoints, UI elements).
- [ ] **Manual Verification Prompt**: Asked user to verify before committing.
- [ ] **User Approval**: Confirmed user explicitly said "commit" (or equivalent) before commit.
- [ ] **Commit Done**: Changes committed with descriptive message.
- [ ] **Todo Updated**: `todo.md` updated if task completed.

## Task Management

- All current and ongoing tasks are stored in `todo.md`.
- Update `todo.md` with new tasks, mark completed tasks, and update timestamps as needed.

## Final Step

- After completing changes, update the list time in `todo.md` to reflect the current timestamp.