-- Seed test data for development
-- This creates sample tasks in different statuses for testing the kanban board

INSERT INTO users (id, email) VALUES (1, 'test@example.com')
ON CONFLICT(id) DO NOTHING;

INSERT INTO backends (id, user_id, backend_type, name)
VALUES (1, 1, 'internal', 'Default Backend')
ON CONFLICT(id) DO NOTHING;

-- Todo tasks
INSERT INTO tasks (backend_id, title, description, status, priority)
VALUES 
(1, 'Setup development environment', 'Install all required dependencies and configure IDE', 'NEEDS-ACTION', 1),
(1, 'Write project documentation', 'Create comprehensive README and API docs', 'NEEDS-ACTION', 2),
(1, 'Review pull requests', 'Check and merge pending PRs from team members', 'NEEDS-ACTION', 3);

-- In Progress tasks
INSERT INTO tasks (backend_id, title, description, status, priority)
VALUES 
(1, 'Implement kanban board', 'Add drag-and-drop functionality to web UI', 'IN-PROCESS', 1),
(1, 'Fix authentication bug', 'Resolve session timeout issues', 'IN-PROCESS', 2);

-- Completed tasks
INSERT INTO tasks (backend_id, title, description, status, priority)
VALUES 
(1, 'Database schema design', 'Define tables and relationships', 'COMPLETED', 1),
(1, 'Initial project setup', 'Create repository and basic structure', 'COMPLETED', 1);
