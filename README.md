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
```bash
./blognotifier --[migrate|explore|crawl]
```
