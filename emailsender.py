import asyncio
from email.message import EmailMessage
from aiosmtplib import SMTP

async def send_email():
    message = EmailMessage()
    message.set_content("Hello, this is a test email.")
    message["From"] = "sender@example.com"
    message["To"] = "recipient@example.com"
    message["Subject"] = "Test Subject"

    async with SMTP(hostname='localhost', port=2500) as smtp:
        await smtp.send_message(message)

if __name__ =='__main__':
    asyncio.run(send_email())
