package caldav

import (
	"crypto/sha256"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/chrisbelyea/momentum/internal/auth"
	"github.com/chrisbelyea/momentum/internal/db"
	"github.com/chrisbelyea/momentum/internal/models"
	"github.com/chrisbelyea/momentum/pkg/vtodo"
)

// Handler handles CalDAV HTTP requests
type Handler struct {
	taskRepo *db.TaskRepository
}

// NewHandler creates a new CalDAV Handler
func NewHandler(taskRepo *db.TaskRepository) *Handler {
	return &Handler{
		taskRepo: taskRepo,
	}
}

// HandleTasks handles requests to /caldav/tasks
// GET: List all tasks for a backend
// POST: Create a new task
func (h *Handler) HandleTasks(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodOptions:
		h.optionsCollection(w)
	case "PROPFIND":
		h.propfindTasks(w, r)
	case "REPORT":
		h.reportTasks(w, r)
	case http.MethodGet:
		h.listTasks(w, r)
	case http.MethodPost:
		h.createTask(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// HandleTask handles requests to /caldav/tasks/{id}
// GET: Get a single task
// PUT: Update a task
// DELETE: Delete a task
func (h *Handler) HandleTask(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		h.optionsResource(w)
		return
	}
	// Extract task ID from path
	taskIDStr := strings.TrimPrefix(r.URL.Path, "/caldav/tasks/")
	if taskIDStr == "" {
		http.Error(w, "Task ID is required", http.StatusBadRequest)
		return
	}

	taskID, err := strconv.Atoi(taskIDStr)
	if err != nil {
		http.Error(w, "Invalid task ID", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		h.getTask(w, r, taskID)
	case http.MethodPut:
		h.updateTask(w, r, taskID)
	case http.MethodDelete:
		h.deleteTask(w, r, taskID)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// listTasks lists all tasks for a backend
func (h *Handler) listTasks(w http.ResponseWriter, r *http.Request) {
	// Get backend_id from query parameter
	backendIDStr := r.URL.Query().Get("backend_id")
	userID, ok := auth.UserIDFromRequest(r)
	if !ok {
		userID = 1
	}
	backendID, err := backendIDForRequest(h.taskRepo, backendIDStr, userID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	tasks, err := h.taskRepo.ListForUser(backendID, userID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to list tasks: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(tasks)
}

// options advertises the subset of WebDAV/CalDAV methods implemented by the
// task collection and its resources. It is intentionally explicit so clients
// do not infer support for REPORT, MKCALENDAR, or other unimplemented methods.
func (h *Handler) optionsCollection(w http.ResponseWriter) {
	w.Header().Set("Allow", "OPTIONS, GET, POST, PROPFIND, REPORT")
	w.Header().Set("DAV", "1, calendar-access")
	w.Header().Set("MS-Author-Via", "DAV")
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) optionsResource(w http.ResponseWriter) {
	w.Header().Set("Allow", "OPTIONS, GET, PUT, DELETE")
	w.Header().Set("DAV", "1, calendar-access")
	w.Header().Set("MS-Author-Via", "DAV")
	w.WriteHeader(http.StatusOK)
}

type propfindMultistatus struct {
	XMLName   xml.Name           `xml:"D:multistatus"`
	XMLNSD    string             `xml:"xmlns:D,attr"`
	XMLNSC    string             `xml:"xmlns:C,attr"`
	Responses []propfindResponse `xml:"D:response"`
}

type propfindResponse struct {
	Href     string           `xml:"D:href"`
	Propstat propfindPropstat `xml:"D:propstat"`
}

type propfindPropstat struct {
	Prop   propfindProperties `xml:"D:prop"`
	Status string             `xml:"D:status"`
}

type propfindProperties struct {
	ResourceType       *propfindResourceType `xml:"D:resourcetype,omitempty"`
	DisplayName        string                `xml:"D:displayname,omitempty"`
	GetETag            string                `xml:"D:getetag,omitempty"`
	GetContentType     string                `xml:"D:getcontenttype,omitempty"`
	SupportedComponent *supportedComponents  `xml:"C:supported-calendar-component-set,omitempty"`
	CalendarData       string                `xml:"C:calendar-data,omitempty"`
}

type propfindResourceType struct {
	Collection *struct{} `xml:"D:collection,omitempty"`
	Calendar   *struct{} `xml:"C:calendar,omitempty"`
}

type supportedComponents struct {
	VTodo *struct{} `xml:"C:comp"`
}

// propfindTasks returns collection metadata and, for Depth: 1, each VTODO
// resource. Unknown depth values are rejected rather than silently returning
// an incomplete view of the collection.
func (h *Handler) propfindTasks(w http.ResponseWriter, r *http.Request) {
	depth := r.Header.Get("Depth")
	if depth == "" {
		depth = "0"
	}
	if depth != "0" && depth != "1" {
		http.Error(w, "unsupported Depth", http.StatusBadRequest)
		return
	}
	userID, ok := auth.UserIDFromRequest(r)
	if !ok {
		userID = 1
	}
	backendID, err := backendIDForRequest(h.taskRepo, r.URL.Query().Get("backend_id"), userID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	tasks, err := h.taskRepo.ListForUser(backendID, userID)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to list tasks: %v", err), http.StatusInternalServerError)
		return
	}
	collectionETag := tasksETag(tasks)
	responses := []propfindResponse{{
		Href: canonicalCollectionHref(r),
		Propstat: propfindPropstat{Status: "HTTP/1.1 200 OK", Prop: propfindProperties{
			ResourceType: &propfindResourceType{Collection: &struct{}{}, Calendar: &struct{}{}},
			DisplayName:  "Momentum Tasks", GetETag: collectionETag,
			SupportedComponent: &supportedComponents{VTodo: &struct{}{}},
		}},
	}}
	if depth == "1" {
		for _, task := range tasks {
			responses = append(responses, propfindResponse{
				Href: fmt.Sprintf("/caldav/tasks/%d", task.ID),
				Propstat: propfindPropstat{Status: "HTTP/1.1 200 OK", Prop: propfindProperties{
					GetETag: taskETag(task), GetContentType: "text/calendar; component=VTODO",
				}},
			})
		}
	}
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.WriteHeader(http.StatusMultiStatus)
	_ = xml.NewEncoder(w).Encode(propfindMultistatus{XMLNSD: "DAV:", XMLNSC: "urn:ietf:params:xml:ns:caldav", Responses: responses})
}

// reportRequest is the common subset of the CalDAV calendar-query and
// calendar-multiget REPORT bodies. A calendar-query has no hrefs; a
// calendar-multiget supplies one or more resource hrefs.
type reportRequest struct {
	XMLName xml.Name
	Hrefs   []string `xml:"href"`
}

// reportTasks implements the read-only CalDAV REPORTs needed by clients to
// enumerate and fetch VTODO resources. It deliberately supports only the
// calendar-query and calendar-multiget report shapes; unsupported filters are
// rejected instead of being silently ignored.
func (h *Handler) reportTasks(w http.ResponseWriter, r *http.Request) {
	var request reportRequest
	decoder := xml.NewDecoder(io.LimitReader(r.Body, 2<<20))
	if err := decoder.Decode(&request); err != nil {
		http.Error(w, "invalid REPORT body", http.StatusBadRequest)
		return
	}
	if request.XMLName.Local != "calendar-query" && request.XMLName.Local != "calendar-multiget" {
		http.Error(w, "unsupported REPORT type", http.StatusBadRequest)
		return
	}
	if request.XMLName.Local == "calendar-query" && len(request.Hrefs) != 0 {
		http.Error(w, "calendar-query must not contain hrefs", http.StatusBadRequest)
		return
	}
	if request.XMLName.Local == "calendar-multiget" && len(request.Hrefs) == 0 {
		http.Error(w, "calendar-multiget requires hrefs", http.StatusBadRequest)
		return
	}

	userID, ok := auth.UserIDFromRequest(r)
	if !ok {
		userID = 1
	}
	backendID, err := backendIDForRequest(h.taskRepo, r.URL.Query().Get("backend_id"), userID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var tasks []*models.Task
	if request.XMLName.Local == "calendar-query" {
		tasks, err = h.taskRepo.ListForUser(backendID, userID)
	} else {
		tasks = make([]*models.Task, 0, len(request.Hrefs))
		for _, href := range request.Hrefs {
			id, parseErr := taskIDFromHref(href)
			if parseErr != nil {
				tasks = append(tasks, nil)
				continue
			}
			task, getErr := h.taskRepo.GetForUser(id, userID)
			if getErr != nil {
				http.Error(w, fmt.Sprintf("failed to fetch task: %v", getErr), http.StatusInternalServerError)
				return
			}
			if task == nil || task.BackendID != backendID {
				tasks = append(tasks, nil)
				continue
			}
			tasks = append(tasks, task)
		}
	}
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to list tasks: %v", err), http.StatusInternalServerError)
		return
	}

	responses := make([]propfindResponse, 0, len(tasks))
	for i, task := range tasks {
		if task == nil {
			href := ""
			if i < len(request.Hrefs) {
				href = request.Hrefs[i]
			}
			responses = append(responses, propfindResponse{Href: href, Propstat: propfindPropstat{Status: "HTTP/1.1 404 Not Found"}})
			continue
		}
		data, marshalErr := todoFromTask(task).Marshal()
		if marshalErr != nil {
			http.Error(w, fmt.Sprintf("failed to serialize task: %v", marshalErr), http.StatusInternalServerError)
			return
		}
		responses = append(responses, propfindResponse{
			Href: fmt.Sprintf("/caldav/tasks/%d", task.ID),
			Propstat: propfindPropstat{Status: "HTTP/1.1 200 OK", Prop: propfindProperties{
				GetETag: taskETag(task), GetContentType: "text/calendar; component=VTODO",
				CalendarData: string(data),
			}},
		})
	}
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.WriteHeader(http.StatusMultiStatus)
	_ = xml.NewEncoder(w).Encode(propfindMultistatus{XMLNSD: "DAV:", XMLNSC: "urn:ietf:params:xml:ns:caldav", Responses: responses})
}

func taskIDFromHref(href string) (int, error) {
	path := href
	if parsed, err := url.Parse(href); err == nil {
		path = parsed.Path
	}
	path = strings.TrimSuffix(path, "/")
	const prefix = "/caldav/tasks/"
	if !strings.HasPrefix(path, prefix) {
		return 0, fmt.Errorf("href is not a task resource")
	}
	id, err := strconv.Atoi(strings.TrimPrefix(path, prefix))
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("href does not identify a task")
	}
	return id, nil
}

func backendIDForRequest(repo *db.TaskRepository, raw string, userID int) (int, error) {
	if raw != "" {
		id, err := strconv.Atoi(raw)
		if err != nil || id <= 0 {
			return 0, fmt.Errorf("invalid backend_id")
		}
		return id, nil
	}
	id, err := repo.DefaultBackendForUser(userID)
	if err != nil {
		return 0, fmt.Errorf("backend_id query parameter is required")
	}
	return id, nil
}

func canonicalCollectionHref(r *http.Request) string {
	if strings.HasSuffix(r.URL.Path, "/") {
		return r.URL.Path
	}
	return r.URL.Path + "/"
}

func tasksETag(tasks []*models.Task) string {
	h := sha256.New()
	for _, task := range tasks {
		_, _ = h.Write([]byte(strconv.Itoa(task.ID)))
		_, _ = h.Write([]byte(taskETag(task)))
	}
	return `"` + fmt.Sprintf("%x", h.Sum(nil)) + `"`
}

// getTask gets a single task by ID
func (h *Handler) getTask(w http.ResponseWriter, r *http.Request, taskID int) {
	userID, ok := auth.UserIDFromRequest(r)
	if !ok {
		userID = 1
	}
	task, err := h.taskRepo.GetForUser(taskID, userID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get task: %v", err), http.StatusInternalServerError)
		return
	}

	if task == nil {
		http.Error(w, "Task not found", http.StatusNotFound)
		return
	}
	if wantsCalendar(r) {
		h.writeCalendarTask(w, r, task)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(task)
}

// createTask creates a new task
func (h *Handler) createTask(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromRequest(r)
	if !ok {
		userID = 1
	}
	var task models.Task
	if isCalendarRequest(r) {
		todo, err := vtodo.Parse(readBody(r))
		if err != nil {
			http.Error(w, fmt.Sprintf("Invalid iCalendar: %v", err), http.StatusBadRequest)
			return
		}
		task = taskFromTodo(todo)
		backendID, err := strconv.Atoi(r.URL.Query().Get("backend_id"))
		if err != nil || backendID == 0 {
			http.Error(w, "backend_id query parameter is required", http.StatusBadRequest)
			return
		}
		task.BackendID = backendID
	} else if err := json.NewDecoder(r.Body).Decode(&task); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
		return
	}

	// Validate required fields
	if task.BackendID == 0 {
		http.Error(w, "backend_id is required", http.StatusBadRequest)
		return
	}
	if task.Title == "" {
		http.Error(w, "title is required", http.StatusBadRequest)
		return
	}
	if task.Status == "" {
		task.Status = models.StatusNeedsAction
	}
	if !h.taskRepo.BackendOwned(task.BackendID, userID) {
		http.Error(w, "backend not found", 404)
		return
	}

	if err := h.taskRepo.Create(&task); err != nil {
		http.Error(w, fmt.Sprintf("Failed to create task: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if isCalendarRequest(r) {
		w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
		w.Header().Set("Location", fmt.Sprintf("/caldav/tasks/%d", task.ID))
		w.Header().Set("ETag", taskETag(&task))
		data, _ := todoFromTask(&task).Marshal()
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write(data)
		return
	}
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(task)
}

// updateTask updates an existing task
func (h *Handler) updateTask(w http.ResponseWriter, r *http.Request, taskID int) {
	userID, ok := auth.UserIDFromRequest(r)
	if !ok {
		userID = 1
	}
	old, err := h.taskRepo.GetForUser(taskID, userID)
	if err != nil || old == nil {
		http.Error(w, "Task not found", http.StatusNotFound)
		return
	}
	if match := r.Header.Get("If-Match"); match != "" && match != taskETag(old) {
		http.Error(w, "ETag does not match", http.StatusPreconditionFailed)
		return
	}
	var task models.Task
	if isCalendarRequest(r) {
		todo, parseErr := vtodo.Parse(readBody(r))
		if parseErr != nil {
			http.Error(w, fmt.Sprintf("Invalid iCalendar: %v", parseErr), http.StatusBadRequest)
			return
		}
		task = taskFromTodo(todo)
		task.BackendID = old.BackendID
	} else if err := json.NewDecoder(r.Body).Decode(&task); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
		return
	}

	// Ensure ID matches the URL
	task.ID = taskID

	// Validate required fields
	if task.BackendID == 0 {
		http.Error(w, "backend_id is required", http.StatusBadRequest)
		return
	}
	if task.Title == "" {
		http.Error(w, "title is required", http.StatusBadRequest)
		return
	}
	if task.Status == "" {
		http.Error(w, "status is required", http.StatusBadRequest)
		return
	}

	if err := h.taskRepo.UpdateForUser(&task, userID); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			http.Error(w, "Task not found", http.StatusNotFound)
			return
		}
		http.Error(w, fmt.Sprintf("Failed to update task: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if isCalendarRequest(r) {
		w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
		w.Header().Set("ETag", taskETag(&task))
		data, _ := todoFromTask(&task).Marshal()
		_, _ = w.Write(data)
		return
	}
	json.NewEncoder(w).Encode(task)
}

// deleteTask deletes a task
func (h *Handler) deleteTask(w http.ResponseWriter, r *http.Request, taskID int) {
	userID, ok := auth.UserIDFromRequest(r)
	if !ok {
		userID = 1
	}
	if current, err := h.taskRepo.GetForUser(taskID, userID); err != nil || current == nil {
		http.Error(w, "Task not found", http.StatusNotFound)
		return
	} else if match := r.Header.Get("If-Match"); match != "" && match != taskETag(current) {
		http.Error(w, "ETag does not match", http.StatusPreconditionFailed)
		return
	}
	if err := h.taskRepo.DeleteForUser(taskID, userID); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			http.Error(w, "Task not found", http.StatusNotFound)
			return
		}
		http.Error(w, fmt.Sprintf("Failed to delete task: %v", err), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
