# Introduction

```
                          ,_         
                        ,'  '\,_     Utku Sen's
                        |_,-'_)         wholeaked
                        /##c '\  (   
                       ' |'  -{.  )  "When you have eliminated the impossible,
                         /\__-' \[]  whatever remains, however improbable,
                        /'-_'\       must be the truth" - Sherlock Holmes
                        '     \    
 ```

wholeaked is a file-sharing tool that allows you to find the responsible person in case of a leakage. It's written in Go.

## How?

wholeaked gets the file that will be shared and a list of recipients. It creates a unique signature for each recipient and adds it to the file secretly. After then, it can automatically send files to the corresponding recipients by using Sendgrid, AWS SES or SMTP integrations. Instead of sending them by e-mail, you can also share them manually.

wholeaked works with every file type. However, it has additional features for common file types such as PDF, DOCX, MOV etc.

### Sharing Process

```                     
                                                       +-----------+
                                                       |Top Secret |
                                                       |.pdf       |       
                                                       |           |       
                                                      -|           |       
                                                     / |           |       
                                                    /  |Hidden     |       
                                             a@gov /   |signature1 |       
                                                  /    +-----------+       
                                                 /     +-----------+       
+-----------++-----------+                      /      |Top Secret |       
|Top Secret ||Recipient  |                     /       |.pdf       |       
|.pdf       ||List       |      +---------+   /        |           |       
|           ||           |      |utkusen/ |  /  b@gov  |           |       
|           ||a@gov      |----->|wholeaked| /----------+           |       
|           ||b@gov      |      |         | \          |Hidden     |       
|           ||c@gov      |      +---------+  \         |signature2 |       
|           ||           |                    \        +-----------+       
+-----------++-----------+                     \       +-----------+       
                                                \      |Top Secret |       
                                                 \     |.pdf       |       
                                           c@gov  \    |           |       
                                                   \   |           |       
                                                    \  |           |       
                                                     \ |Hidden     |       
                                                      -|signature3 |       
                                                       +-----------+    
```

### Validation Part

To find who leaked the document, you just need to provide the leaked file to wholeaked, and it will reveal the responsible person by comparing the signatures in the database.

```
+-----------+             +---------+                           
|Top Secret |             |Signature|                           
|.pdf       |  +---------+|Database |                           
|           |  |utkusen/ ||         |         Document leaked by
|           |->|wholeaked||         |--------+                  
|           |  |         ||         |              b@gov        
|Hidden     |  +---------+|         |                           
|Signature2 |             |         |                           
+-----------+             +---------+                           
                                                               
```

## Demonstration Video

[![Demo Video](https://img.youtube.com/vi/EEDtXp9ngHw/0.jpg)](https://www.youtube.com/watch?v=EEDtXp9ngHw)

## File Types and Detection Modes

wholeaked can add the unique signature to different sections of a file. Available detection modes are given below:

**File Hash:** SHA256 hash of the file. All file types are supported. 

**Binary:** The signature is directly added to the binary. *Almost* all file types are supported.

**Metadata:** The signature is added to a metadata section of a file. Supported file types: PDF, DOCX, XLSX, PPTX, MOV, JPG, PNG, GIF, EPS, AI, PSD

**Watermark:** An invisible signature is inserted into the text. Only PDF files are supported.

# Installation

## From Binary

You can download the pre-built binaries from the [releases](https://github.com/utkusen/wholeaked/releases/latest) page and run. For example:

`unzip wholeaked_0.1.0_macOS_amd64.zip`

`./wholeaked --help`

## From Source

1) Install Go on your system

2) Run: `go install github.com/utkusen/wholeaked@latest`

## Installing Dependencies

wholeaked requires `exiftool` for adding signatures to metadata section of files. If you don't want to use this feature, you don't need to install it.

1) Debian-based Linux: Run `apt install exiftool`
2) macOS: Run `brew install exiftool`
3) Windows: Download exiftool from here https://exiftool.org/ and put the `exiftool.exe` in the same directory with wholeaked.

wholeaked requires `pdftotext` for verifying watermarks inside PDF files. If you don't want to use this feature, you don't need to install it.

1) Download "Xpdf command line tools" for Linux, macOS or Windows from here: https://www.xpdfreader.com/download.html
2) Extract the archive and navigate to `bin64` folder.
3) Copy the `pdftotext` (or `pdftotext.exe`) executable to the same folder with wholeaked
4) For Debian Based Linux: Run `apt install libfontconfig` command.

# Usage

## Basic Usage

wholeaked requires a project name `-n`, the path of the base file which the signatures will add `-f` and a list of target recipients `-t`

Example command: `./wholeaked -n test_project -f secret.pdf -t targets.txt`

The `targets.txt` file should contain name and the e-mail address in the following format:

```
Utku Sen,utku@utkusen.com
Bill Gates,bill@microsoft.com
```

After execution is completed, the following unique files will be generated:

```
test_project/files/Utku_Sen/secret.pdf
test_project/files/Bill_Gates/secret.pdf
```

By default, wholeaked adds signatures to all available places that are defined in the "File Types and Detection Modes" section. If you don't want to use a method, you can define it with a `false` flag. For example:

`./wholeaked -n test_project -f secret.pdf -t targets.txt -binary=false -metadata=false -watermark=false`

## Sending E-mails

In order to send e-mails, you need to fill some sections in the `CONFIG` file.

- If you want to send e-mails via Sendgrid, type your API key to the `SENDGRID_API_KEY` section. 

- If you want to send e-mails via AWS SES integration, you need to install `awscli` on your machine and add the required AWS key to it. wholeaked will read the key by itself. But you need to fill the `AWS_REGION` section in the config file. 

- If you want to send e-mails via a SMTP server, fill the `SMTP_SERVER`, `SMTP_PORT`, `SMTP_USERNAME`, `SMTP_PASSWORD` sections.

The other necessary fields to fill:

- `EMAIL_TEMPLATE_PATH` Path of the e-mail's body. You can specify use HTML or text format.
- `EMAIL_CONTENT_TYPE` Can be `html` or `text`
- `EMAIL_SUBJECT` Subject of the e-mail
- `FROM_NAME` From name of the e-mail
- `FROM_EMAIL` From e-mail of the e-mail

To specify the sending method, you can use `-sendgrid`, `-ses` or `-smtp` flags. For example:

`./wholeaked -n test_project -f secret.pdf -t targets.txt -sendgrid`

## Validating a Leaked File

You can use the `-validate` flag to reveal the owner of a leaked file. wholeaked will compare the signatures detected in the file and the database located in the project folder. Example:

`./wholeaked -n test_project -f secret.pdf -validate`

**Important:** You shouldn't delete the `project_folder/db.csv` file if you want to use the file validation feature. If that file is deleted, wholeaked won't be able to compare the signatures.

# Donation

Loved the project? You can buy me a coffee

<a href="https://www.buymeacoffee.com/utkusen" target="_blank"><img src="https://cdn.buymeacoffee.com/buttons/default-orange.png" alt="Buy Me A Coffee" height="41" width="174"></a>
