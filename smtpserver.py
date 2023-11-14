import asyncio
from aiosmtpd.controller import Controller


class MyHandler:
    def __init__(self):
        self.inbox = {}
    
    async def handle_DATA(self, server, session, envelope):
        print('Message from %s' % envelope.mail_from)
        print('Message for %s' % envelope.rcpt_tos)
        print('Message data:\n%s' % envelope.content.decode('utf8', errors='replace'))
        if self.inbox.get(envelope.rcpt_tos[0]) is None:
            self.inbox[envelope.rcpt_tos[0]] = [{
                "from": envelope.mail_from,
                "to": envelope.rcpt_tos,
                "messages": envelope.content.decode('utf8', errors='replace')
            }]
        else:
            self.inbox[envelope.rcpt_tos[0]].append({
                "from": envelope.mail_from,
                "to": envelope.rcpt_tos,
                "messages": envelope.content.decode('utf8', errors='replace')
            }) 
        print(self.inbox)
        return '250 Message accepted for delivery'

async def start_smtp_server():
    handler = MyHandler()
    controller = Controller(handler, hostname='127.0.0.1', port=2500)
    controller.start()

if __name__ == '__main__':
    asyncio.run(start_smtp_server())
