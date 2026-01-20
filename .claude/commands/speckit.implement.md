---
description: Execute the implementation plan by processing and executing all tasks defined in tasks.md
---

## User Input

```text
$ARGUMENTS
```

## Outline

1. Run `.specify/scripts/bash/check-prerequisites.sh --json --require-tasks --include-tasks` from repo root and parse FEATURE_DIR and AVAILABLE_DOCS list.

2. **Check checklists status** (if FEATURE_DIR/checklists/ exists):
   - Scan all checklist files in the checklists/ directory
   - For each checklist, count total/completed/incomplete items
   - Create a status table:
     ```text
     | Checklist | Total | Completed | Incomplete | Status |
     ```
   - If any checklist is incomplete:
     - Display the table with incomplete item counts
     - Ask: "Some checklists are incomplete. Proceed with implementation? (yes/no)"
     - Wait for user response before continuing

3. Load and analyze the implementation context:
   - **REQUIRED**: Read tasks.md for the complete task list and execution plan
   - **REQUIRED**: Read plan.md for tech stack, architecture, and file structure
   - **IF EXISTS**: Read data-model.md for entities and relationships
   - **IF EXISTS**: Read contracts/ for API specifications
   - **IF EXISTS**: Read research.md for technical decisions

4. **Project Setup Verification**:
   - Check if repository is a git repo and create/verify .gitignore
   - Check for Dockerfile* → create/verify .dockerignore
   - Check for .eslintrc* → create/verify .eslintignore
   - Check for .prettierrc* → create/verify .prettierignore
   - Check for terraform files → create/verify .terraformignore

5. Parse tasks.md structure and extract:
   - Task phases: Setup, Tests, Core, Integration, Polish
   - Task dependencies: Sequential vs parallel execution rules
   - Task details: ID, description, file paths, parallel markers [P]
   - Execution flow: Order and dependency requirements

6. Execute implementation following the task plan:
   - Phase-by-phase execution: Complete each phase before moving to the next
   - Respect dependencies: Run sequential tasks in order, parallel tasks [P] can run together
   - Follow TDD approach: Execute test tasks before their corresponding implementation tasks
   - File-based coordination: Tasks affecting the same files must run sequentially

7. Progress tracking and error handling:
   - Report progress after each completed task
   - Halt execution if any non-parallel task fails
   - For parallel tasks [P], continue with successful tasks, report failed ones
   - **IMPORTANT**: Mark completed tasks as [X] in the tasks file

8. Completion validation:
   - Verify all required tasks are completed
   - Check that implemented features match the original specification
   - Validate that tests pass and coverage meets requirements
   - Report final status with summary of completed work