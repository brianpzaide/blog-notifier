import http.server
import socketserver
import multiprocessing
import subprocess
import time

# Define the request handler for the HTTP server
handler = http.server.SimpleHTTPRequestHandler

def run_http_server(port):
    # Create an HTTP server with the specified port and handler
    with socketserver.TCPServer(("", port), handler) as httpd:
        print(f"Serving on port {port}")
        httpd.serve_forever()

def run_blog_notifier():
    try:
        subprocess.run(["sh", "blog_notifier.sh"], check=True)
    except subprocess.CalledProcessError as e:
        print(f"Error running blog_notifier.sh: {e}")

def run_inbucket_server():
    try:
        subprocess.run(["/start-inbucket.sh"], check=True)
    except subprocess.CalledProcessError as e:
        print(f"Error running inbucket server: {e}")

if __name__ == "__main__":
    # Define the number of server processes to run concurrently
    httpserver_process = multiprocessing.Process(target=run_http_server, args=(8000,))
    blognotifier_process = multiprocessing.Process(target=run_blog_notifier, args=())
    inbucket_process = multiprocessing.Process(target=run_inbucket_server, args=())
    httpserver_process.start()
    inbucket_process.start()
    time.sleep(10)
    blognotifier_process.start()