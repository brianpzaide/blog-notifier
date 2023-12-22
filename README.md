## Go Sample solution for Blog Notifier
### To build
```bash
>>> go build -o blognotifier .
```
### To run
```aiosmtpd``` is great for running the SMTP server locally.
In one terminal run the following command to run pythons's ```aiosmtpd``` server.
```bash 
aiosmtpd -nl "127.0.0.1:25000"
``` 

In the second terminal run the python's ```http.server``` module
```bash
python3 -m http.server
```
in the third terminal run the blognotifier.go programm
try the following commands
create tables
```bash
./blognotifier --migrate
```
add a site to watch list
```bash
./blognotifier --explore "http://localhost:8000/fake-blog"
```
crawl all the sites that are in the watchlist
```bash
./blognotifier --crawl
```
after the above command check whether new posts that are discovered are inserted to the database and also check if the mails are sent.

add a new site to the watchlist
```bash
./blognotifier --explore "http://localhost:8000/fake-blog2"
```
crawl all the sites that are in the watchlist
```bash
./blognotifier --crawl
```
check that only the posts from the new sites should be added to the database and notifications only for the blog posts that are in the new site are sent to the user.

remove a site
```bash
./blognotifier --remove "http://localhost:8000/fake-blog"
```







