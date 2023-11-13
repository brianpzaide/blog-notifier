#!/bin/bash
python3 blog_notifier.py -migrate
python3 blog_notifier.py -explore "http://localhost:8000/dummy-blog/hello.html"
python3 blog_notifier.py -crawl