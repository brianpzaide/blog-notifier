#!/bin/bash
python3 blog_notifier.py -migrate
python3 blog_notifier.py -explore "https://hyperskill.org/blog/"
python3 blog_notifier.py -crawl