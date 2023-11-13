FROM inbucket/inbucket:latest

# Install Python and any other necessary packages
ENV PYTHONUNBUFFERED=1
RUN apk add --update --no-cache python3 && ln -sf python3 /usr/bin/python
RUN python3 -m ensurepip
RUN pip3 install --no-cache --upgrade pip setuptools

# Expose ports (SMTP HTTP POP3)
EXPOSE 2500 9000 1100

COPY requirements.txt .
RUN pip install -r requirements.txt

RUN mkdir dummy-blog
COPY dummy-blog dummy-blog/
COPY credentials.yml .
COPY blog_notifier.sh .
RUN chmod +x blog_notifier.sh
COPY start.py .


ENTRYPOINT ["python3", "start.py"]
