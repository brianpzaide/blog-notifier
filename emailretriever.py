import imaplib
import email
from email.header import decode_header

def decode_subject(header):
    value, charset = decode_header(header)[0]
    if isinstance(value, bytes):
        return value.decode(charset or "utf-8")
    return value

def retrieve_email():
    server = "127.0.0.1:2500"
    username = "recipient@example.com"
    password = "your-password"

    # Connect to the IMAP server
    connection = imaplib.IMAP4(host="127.0.0.1",port="2500")
    connection.login(username, password)
    connection.select("INBOX")

    # Search for all emails
    _, messages = connection.search(None, "ALL")
    message_ids = messages[0].split()

    for message_id in message_ids:
        _, msg_data = connection.fetch(message_id, "(RFC822)")
        raw_email = msg_data[0][1]
        msg = email.message_from_bytes(raw_email)

        # Get email details
        subject = decode_subject(msg["Subject"])
        sender = msg["From"]
        content_type = msg.get_content_type()

        print(f"Subject: {subject}")
        print(f"From: {sender}")
        print(f"Content Type: {content_type}")

        # Print the email body
        if msg.is_multipart():
            for part in msg.walk():
                if part.get_content_type() == "text/plain":
                    body = part.get_payload(decode=True)
                    print(f"Body:\n{body.decode('utf-8', errors='replace')}")
        else:
            body = msg.get_payload(decode=True)
            print(f"Body:\n{body.decode('utf-8', errors='replace')}")

        print("-" * 50)

    # Logout and close the connection
    connection.logout()

if __name__ == '__main__':
	retrieve_email()
