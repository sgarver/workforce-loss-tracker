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
   - **DO NOT PROCEED WITHOUT EXPLICIT USER APPROVAL**. Wait for confirmation before advancing to step 5.

5. **Commit to Git**:
   - **ONLY AFTER USER APPROVAL**: Commit the changes: `git add .` and `git commit -m "Description of changes"`
   - Ask the user about the next task.

## Task Management

- All current and ongoing tasks are stored in `todo.md`.
- Update `todo.md` with new tasks, mark completed tasks, and update timestamps as needed.

## Final Step

- After completing changes, update the list time in `todo.md` to reflect the current timestamp.