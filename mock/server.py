from http.server import BaseHTTPRequestHandler, HTTPServer

class Handler(BaseHTTPRequestHandler):
    def do_GET(self):
        self.send_response(200)
        self.send_header("Content-Type", "text/plain")
        self.end_headers()

        for name, value in self.headers.items():
            self.wfile.write(f"{name}: {value}\n".encode())

HTTPServer(("0.0.0.0", 8082), Handler).serve_forever()
