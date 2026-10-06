// Momentum Linux client: HTTPS-only GTK interface to the versioned server API.
using Gtk;

namespace Momentum {
    private const string MEDIA = "application/vnd.momentum.v1+json";
    private const string SCHEMA_NAME = "org.momentum.credentials";
    private class Client : Gtk.Application {
        private Secret.Schema session_schema = new Secret.Schema (SCHEMA_NAME, Secret.SchemaFlags.NONE,
            "origin", Secret.SchemaAttributeType.STRING,
            "account", Secret.SchemaAttributeType.STRING,
            "type", Secret.SchemaAttributeType.STRING);
        private Gtk.ApplicationWindow window;
        private Gtk.Entry server;
        private Gtk.Entry account;
        private Gtk.PasswordEntry password;
        private Gtk.Label notice;
        private Gtk.Box tasks_box;
        private Gtk.Entry title;
        private Gtk.DropDown view;
        private Soup.Session http;
        private string origin = "";
        private string user = "";
        private string? cookie = null;
        private Json.Array? tasks;

        public Client () {
            Object (application_id: "org.momentum.Linux");
            http = new Soup.Session ();
            http.timeout = 15;
        }

        private bool set_origin (string value) {
            // Discovery is deliberately limited to the local packaged server.
            // Remote endpoints require an explicitly entered HTTPS origin and a
            // normally trusted certificate; no TLS exception or HTTP fallback.
            try {
                var uri = GLib.Uri.parse (value, GLib.UriFlags.NONE);
                if (uri.get_scheme () != "https" || uri.get_host () == null ||
                    uri.get_userinfo () != null || uri.get_query () != null ||
                    uri.get_fragment () != null || (uri.get_path () != "" && uri.get_path () != "/"))
                    return false;
                origin = value.has_suffix ("/") ? value.substring (0, value.length - 1) : value;
                return true;
            } catch (Error e) {
                return false;
            }
        }

        private Json.Node? request (string method, string path, string? body = null, string? version = null) throws Error {
            var message = new Soup.Message (method, origin + path);
            message.request_headers.append ("Accept", MEDIA);
            if (cookie != null) message.request_headers.append ("Cookie", cookie);
            if (version != null) message.request_headers.append ("If-Match", "\"" + version + "\"");
            if (body != null) {
                message.set_request_body_from_bytes ("application/json", new Bytes (body.data));
            }
            var bytes = http.send_and_read (message, null);
            uint code = message.status_code;
            if (path == "/auth/login" && code == 200) {
                string? header = message.response_headers.get_one ("Set-Cookie");
                if (header == null || !header.has_prefix ("momentum_session="))
                    throw new IOError.FAILED ("Login returned no session cookie");
                cookie = header.split (";", 2)[0];
            }
            if (code == 401) throw new IOError.PERMISSION_DENIED ("Session expired; sign in again");
            if (code == 409) throw new IOError.FAILED ("Task changed on the server; refresh before retrying");
            if (code < 200 || code >= 300) throw new IOError.FAILED ("Server returned HTTP %u".printf (code));
            if (code == 204) return null;
            var parser = new Json.Parser ();
            parser.load_from_data ((string) bytes.get_data (), (ssize_t) bytes.get_size ());
            return parser.get_root ().copy ();
        }

        private void announce (string message) { notice.label = message; }

        private void connect_server () {
            if (!set_origin (server.text)) {
                announce ("Enter an HTTPS server origin (default: https://127.0.0.1:8443)");
                return;
            }
            user = account.text.strip ();
            if (user == "" || password.text == "") {
                announce ("Enter account and password");
                return;
            }
            try {
                var obj = new Json.Object ();
                obj.set_string_member ("Email", user);
                obj.set_string_member ("Password", password.text);
                var node = new Json.Node (Json.NodeType.OBJECT);
                node.set_object (obj);
                var generator = new Json.Generator ();
                generator.set_root (node);
                request ("POST", "/auth/login", generator.to_data (null));
                password.text = "";
                var capabilities = request ("GET", "/api/v1/capabilities");
                var api = capabilities.get_object ().get_object_member ("api");
                if (api.get_int_member ("minimum") > 1 || api.get_int_member ("maximum") < 1) {
                    // Never retain a session for an incompatible server.
                    request ("POST", "/auth/logout");
                    cookie = null;
                    announce ("Unsupported server API version. Upgrade Momentum Linux or the server.");
                    return;
                }
                Secret.password_store_sync (session_schema, Secret.COLLECTION_DEFAULT,
                    "Momentum session", cookie, null,
                    "origin", origin, "account", user, "type", "session");
                announce ("Connected: " + origin);
                refresh ();
            } catch (Error e) {
                password.text = "";
                cookie = null;
                announce (e.message);
            }
        }

        private void restore () {
            if (!set_origin (server.text) || account.text.strip () == "") return;
            user = account.text.strip ();
            try {
                cookie = Secret.password_lookup_sync (session_schema, null,
                    "origin", origin, "account", user, "type", "session");
                if (cookie != null) {
                    var capabilities = request ("GET", "/api/v1/capabilities");
                    var api = capabilities.get_object ().get_object_member ("api");
                    if (api.get_int_member ("minimum") > 1 || api.get_int_member ("maximum") < 1)
                        throw new IOError.NOT_SUPPORTED ("Server API v1 is not available");
                    refresh ();
                }
            } catch (Error e) {
                cookie = null;
                announce (e.message);
            }
        }

        private void logout () {
            if (cookie == null) return;
            try {
                request ("POST", "/auth/logout");
                Secret.password_clear_sync (session_schema, null,
                    "origin", origin, "account", user, "type", "session");
                cookie = null;
                tasks = null;
                render ();
                announce ("Signed out; session revoked");
            } catch (Error e) {
                // Do not claim server revocation when it failed. Leave keychain
                // record intact so a retry can invalidate this session.
                announce ("Logout failed; retry to revoke session: " + e.message);
            }
        }

        private void refresh () {
            if (cookie == null) { announce ("Sign in first"); return; }
            try {
                var result = request ("GET", "/api/v1/tasks");
                tasks = result.get_object ().get_array_member ("tasks").ref ();
                var conflicts = request ("GET", "/api/v1/sync/conflicts");
                uint n = conflicts.get_array ().get_length ();
                announce ("Connected · %u open sync conflicts".printf (n));
                render ();
            } catch (Error e) { announce (e.message); }
        }

        private void mutate (string method, string path, string? body = null, string? version = null) {
            try {
                request (method, path, body, version);
                refresh ();
            } catch (Error e) {
                announce (e.message);
                if (e.message.contains ("changed on the server")) refresh ();
            }
        }

        private void create_task () {
            if (cookie == null || title.text.strip () == "") return;
            var obj = new Json.Object ();
            obj.set_string_member ("title", title.text.strip ());
            var node = new Json.Node (Json.NodeType.OBJECT);
            node.set_object (obj);
            var generator = new Json.Generator ();
            generator.set_root (node);
            mutate ("POST", "/api/v1/tasks", generator.to_data (null));
            title.text = "";
        }

        private void render () {
            Gtk.Widget? child;
            while ((child = tasks_box.get_first_child ()) != null) tasks_box.remove (child);
            if (tasks == null) return;
            for (uint i = 0; i < tasks.get_length (); i++) {
                var task = tasks.get_object_element (i);
                string status = task.get_string_member ("status");
                if (view.get_selected () == 0 && status == "CANCELLED") continue;
                string name = task.get_string_member ("title");
                int64 id = task.get_int_member ("id");
                string version = task.get_string_member ("updated_at");
                var row = new Gtk.Box (Gtk.Orientation.HORIZONTAL, 8);
                row.append (new Gtk.Label ("%s · %s".printf (status, name)));
                var next = new Gtk.Button.with_label (status == "COMPLETED" ? "Reopen" : "Advance");
                string target = status == "NEEDS-ACTION" ? "IN-PROCESS" : status == "IN-PROCESS" ? "COMPLETED" : "NEEDS-ACTION";
                next.clicked.connect (() => mutate ("PATCH", "/api/v1/tasks/%lld/status".printf (id),
                    "{\"status\":\"%s\"}".printf (target), version));
                row.append (next);
                var remove = new Gtk.Button.with_label ("Delete");
                remove.clicked.connect (() => mutate ("DELETE", "/api/v1/tasks/%lld".printf (id), null, version));
                row.append (remove);
                tasks_box.append (row);
            }
        }

        protected override void activate () {
            window = new Gtk.ApplicationWindow (this);
            window.title = "Momentum";
            window.set_default_size (780, 500);
            var page = new Gtk.Box (Gtk.Orientation.VERTICAL, 10);
            page.margin_top = page.margin_bottom = page.margin_start = page.margin_end = 16;
            server = new Gtk.Entry ();
            server.text = "https://127.0.0.1:8443";
            server.placeholder_text = "HTTPS server origin";
            page.append (server);
            account = new Gtk.Entry ();
            account.placeholder_text = "Account email";
            page.append (account);
            password = new Gtk.PasswordEntry ();
            password.placeholder_text = "Password (not saved)";
            page.append (password);
            var actions = new Gtk.Box (Gtk.Orientation.HORIZONTAL, 8);
            var login = new Gtk.Button.with_label ("Sign in");
            login.clicked.connect (connect_server);
            actions.append (login);
            var resume = new Gtk.Button.with_label ("Restore session");
            resume.clicked.connect (restore);
            actions.append (resume);
            var signout = new Gtk.Button.with_label ("Sign out");
            signout.clicked.connect (logout);
            actions.append (signout);
            page.append (actions);
            notice = new Gtk.Label ("Connect to your Momentum server");
            page.append (notice);
            view = new Gtk.DropDown.from_strings ({"Board", "List"});
            view.notify["selected"].connect (render);
            page.append (view);
            var editor = new Gtk.Box (Gtk.Orientation.HORIZONTAL, 8);
            title = new Gtk.Entry ();
            title.placeholder_text = "New task title";
            editor.append (title);
            var add = new Gtk.Button.with_label ("Add task");
            add.clicked.connect (create_task);
            editor.append (add);
            var reload = new Gtk.Button.with_label ("Refresh / sync status");
            reload.clicked.connect (refresh);
            editor.append (reload);
            page.append (editor);
            var scroll = new Gtk.ScrolledWindow ();
            scroll.vexpand = true;
            tasks_box = new Gtk.Box (Gtk.Orientation.VERTICAL, 8);
            scroll.set_child (tasks_box);
            page.append (scroll);
            window.set_child (page);
            window.present ();
        }
    }

    public static int main (string[] args) {
        return new Client ().run (args);
    }
}
