# Query Plan Improvements - 002-indexes

This document demonstrates the performance improvements from the indexes created in changeset `002-indexes`.

## Indexes Created

### 1. Composite Index: `idx_tasks_backend_status`
- **Columns**: `(backend_id, status)`
- **Purpose**: Optimize queries filtering tasks by backend and status, which is the most common query pattern for board/list views

### 2. Single Column Index: `idx_tasks_due_at`
- **Column**: `due_at`
- **Purpose**: Optimize queries filtering or sorting by due date

## Query Plan Analysis

### Query 1: Filter by backend_id AND status
**Use Case**: Display all "in-progress" tasks from a specific backend (most common board query)

```sql
SELECT * FROM tasks WHERE backend_id = 1 AND status = 'todo';
```

**Query Plan**:
```
SEARCH tasks USING INDEX idx_tasks_backend_status (backend_id=? AND status=?)
```

**Result**: ✅ The composite index is used efficiently for both columns.

---

### Query 2: Filter by backend_id only
**Use Case**: Display all tasks from a specific backend (multi-status view)

```sql
SELECT * FROM tasks WHERE backend_id = 1;
```

**Query Plan**:
```
SEARCH tasks USING INDEX idx_tasks_backend_status (backend_id=?)
```

**Result**: ✅ The composite index is used efficiently for prefix matching.

---

### Query 3: Filter by status only
**Use Case**: Display all tasks with a specific status across all backends

```sql
SELECT * FROM tasks WHERE status = 'in-progress';
```

**Query Plan**:
```
SCAN tasks
```

**Result**: ⚠️ Full table scan (expected behavior - composite index requires backend_id for optimal use)

**Note**: This query pattern is less common. If needed in the future, a separate single-column index on `status` can be added. For now, this is acceptable given the priority on backend-filtered queries.

---

### Query 4: Order by due_at
**Use Case**: Display tasks sorted by due date (upcoming/overdue views)

```sql
SELECT * FROM tasks ORDER BY due_at;
```

**Query Plan**:
```
SCAN tasks USING INDEX idx_tasks_due_at
```

**Result**: ✅ The due_at index is used for efficient sorting.

---

### Query 5: Filter by due_at range
**Use Case**: Display tasks due within a specific date range

```sql
SELECT * FROM tasks 
WHERE due_at >= '2024-12-22 00:00:00' 
  AND due_at <= '2024-12-25 00:00:00';
```

**Query Plan**:
```
SEARCH tasks USING INDEX idx_tasks_due_at (due_at>? AND due_at<?)
```

**Result**: ✅ The due_at index is used efficiently for range queries.

## Performance Impact

With these indexes in place:

1. **Board View Queries** (backend_id + status filtering):
   - Time complexity: O(log n + k) where k is matching rows
   - Without index: O(n) full table scan
   - **Expected improvement**: 10-100x faster with 1,000+ tasks

2. **Due Date Queries** (sorting/filtering by due_at):
   - Time complexity: O(log n + k) for range queries
   - Without index: O(n log n) for sorting
   - **Expected improvement**: 5-50x faster with 1,000+ tasks

3. **Memory Usage**:
   - Additional storage: ~2-5% of table size per index
   - Trade-off: Acceptable for query performance gains

## Specification Compliance

These indexes satisfy the performance requirements from:
- **Section 6 (Non-Functional Requirements)**: "Board interactions remain responsive with 1,000+ tasks"
- **Section 10 (Data Model Overview)**: Optimized for common query patterns on backend and status fields

## Testing

Tested on:
- ✅ SQLite (default for self-hosted)
- ⚠️ PostgreSQL validation pending (but schema is cross-compatible)

## References

- Issue: Performance indexes: 002-indexes
- Changeset: `liquibase/changelog/003-indexes.yaml`
- Specification: `requirements/specification.md` (Sections 6, 10)
