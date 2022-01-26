package main

import (
	"archive/zip"
	"bufio"
	"bytes"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ses"
	"github.com/barasher/go-exiftool"
	"github.com/fatih/color"
	"github.com/google/uuid"
	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
	"github.com/sendgrid/sendgrid-go"
	"github.com/sendgrid/sendgrid-go/helpers/mail"
	"gopkg.in/gomail.v2"
)

var currentDir, _ = os.Getwd()

func main() {
	fmt.Println(`
      ,_         
    ,'  '\,_     Utku Sen's
    |_,-'_)         wholeaked
    /##c '\  (   
   ' |'  -{.  )  "When you have eliminated the impossible,
     /\__-' \[]  whatever remains, however improbable,
    /'-_'\       must be the truth" - Sherlock Holmes
    '     \    
 `)

	projectName := flag.String("n", "", "Name of the project")
	targetsFile := flag.String("t", "", "Path of the targets file")
	baseFile := flag.String("f", "", "Path of the base file")
	binaryFlag := flag.Bool("binary", true, "Add a unique signature to the binary")
	metadataFlag := flag.Bool("metadata", true, "Add a unique signature to metadata of the file")
	watermarkFlag := flag.Bool("watermark", true, "Add a unique signature to the PDF file as a watermark")
	sendgridFlag := flag.Bool("sendgrid", false, "Send files with Sendgrid Integration")
	sesFlag := flag.Bool("ses", false, "Send files with AWS SES Integration")
	smtpFlag := flag.Bool("smtp", false, "Send files with a SMTP server")
	validateFlag := flag.Bool("validate", false, "Find who leaked the file")
	flag.Parse()
	if *projectName == "" {
		color.Red("Project name (-n) is required.")
		flag.PrintDefaults()
		os.Exit(1)
	}
	if *targetsFile == "" && !*validateFlag {
		color.Red("Targets file (-t) is required.")
		flag.PrintDefaults()
		os.Exit(1)
	}
	if *baseFile == "" {
		color.Red("Base file (-f) is required.")
		flag.PrintDefaults()
		os.Exit(1)
	}

	if !*binaryFlag && !*metadataFlag && !*watermarkFlag && !*validateFlag {
		color.Red("No flags are set")
		os.Exit(1)
	}
	startProcess(*baseFile, *targetsFile, *projectName, *binaryFlag, *metadataFlag, *watermarkFlag, *sendgridFlag, *sesFlag, *smtpFlag, *validateFlag)

}

func startProcess(baseFile string, targetsFile string, projectName string, binaryFlag bool, metadataFlag bool, watermarkFlag bool, sendgridFlag bool, sesFlag bool, smtpFlag bool, validateFlag bool) {
	fmt.Println("Operation started")
	projectDir := filepath.Join(currentDir, projectName)
	dbPath := filepath.Join(projectDir, "db.csv")
	existsFlag := false
	if validateFlag {
		detectLeak(baseFile, dbPath)
		return
	}
	if _, err := os.Stat(targetsFile); os.IsNotExist(err) {
		color.Red("Targets file does not exist.")
		os.Exit(1)
	}

	if _, err := os.Stat(baseFile); os.IsNotExist(err) {
		color.Red("Targets file does not exist.")
		os.Exit(1)
	}
	err := os.Mkdir(projectDir, 0777)
	if err != nil {
		if !sendgridFlag && !sesFlag && !smtpFlag {
			color.Red("Project already exists.")
			os.Exit(1)
		} else {
			fmt.Println("Local files are already created for this project. Do you want to send them? (y/n)")
			reader := bufio.NewReader(os.Stdin)
			input, err := reader.ReadString('\n')
			if err != nil {
				fmt.Println("An error occured while reading input. Please try again", err)
				os.Exit(1)
			}
			input = strings.TrimSuffix(input, "\n")
			if input != "y" {
				os.Exit(1)
			} else {
				existsFlag = true
			}
		}
	}
	if !existsFlag {
		generateTargetDB(dbPath, readTargets(targetsFile))
		createLocalFiles(baseFile, projectName, binaryFlag, metadataFlag, watermarkFlag)
		color.Magenta("Local files are created")
	}
	configs := parseConfigFile()

	if sendgridFlag {
		fmt.Println("Sending files with Sendgrid")
		for _, line := range readTargets(dbPath) {
			sendWithSendgrid(strings.Split(line, ",")[0], strings.Split(line, ",")[1], configs["FROM_NAME"], configs["FROM_EMAIL"], configs["EMAIL_SUBJECT"], configs["EMAIL_TEMPLATE_PATH"], configs["EMAIL_CONTENT_TYPE"], strings.Split(line, ",")[4])
		}
	}
	if sesFlag {
		fmt.Println("Sending files with AWS SES")
		for _, line := range readTargets(dbPath) {
			sendWithSES(strings.Split(line, ",")[0], strings.Split(line, ",")[1], configs["FROM_NAME"], configs["FROM_EMAIL"], configs["EMAIL_SUBJECT"], configs["EMAIL_TEMPLATE_PATH"], configs["EMAIL_CONTENT_TYPE"], strings.Split(line, ",")[4], configs["AWS_REGION"])
		}
	}
	if smtpFlag {
		fmt.Println("Sending files with the SMTP Server")
		for _, line := range readTargets(dbPath) {
			sendWithSMTP(strings.Split(line, ",")[0], strings.Split(line, ",")[1], configs["FROM_NAME"], configs["FROM_EMAIL"], configs["EMAIL_SUBJECT"], configs["EMAIL_TEMPLATE_PATH"], configs["EMAIL_CONTENT_TYPE"], strings.Split(line, ",")[4])
		}
	}

	fmt.Println("Operation Done. Bye")

}

func detectWatermarkPDF(file string, signature string) bool {
	operatingSystem := runtime.GOOS
	var cmd *exec.Cmd
	if operatingSystem == "windows" {
		cmd = exec.Command(filepath.Join(currentDir, "pdftotext.exe"), file)
	} else {
		cmd = exec.Command(filepath.Join(currentDir, "pdftotext"), file)
	}
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		color.Red("Couldn't run pdftotext binary")
		fmt.Println(err)
		os.Exit(1)
	}
	outPath := strings.Replace(file, ".pdf", ".txt", -1)
	content, err := ioutil.ReadFile(outPath)
	if err != nil {
		color.Red("Couldn't read the PDF file")
		fmt.Println(err)
		os.Exit(1)
	}

	text := string(content)
	os.Remove(outPath)
	return strings.Contains(text, signature)
}

func addWatermarkPDF(file string, signature string) {
	onTop := false
	update := false
	wm, _ := api.TextWatermark(signature, "sc:.9, rot:0, mo:1, op:0", onTop, update, pdfcpu.POINTS)
	api.AddWatermarksFile(file, "", nil, wm, nil)
}

func detectLeak(file string, dbPath string) {
	targets := readTargets(dbPath)
	foundFlag := false
	for _, target := range targets {
		signature := strings.Split(target, ",")[2]
		name := strings.Replace(strings.Split(target, ",")[0], " ", "_", -1)
		fileHash := strings.Split(target, ",")[3]
		binaryFlag, hashFlag, metadataFlag, watermarkFlag := detectSignature(file, signature, fileHash)
		if hashFlag {
			color.Magenta("File Hash Matched: " + name)
			foundFlag = true
		}
		if binaryFlag {
			color.Magenta("Signature Detected in Binary: " + name)
			foundFlag = true
		}
		if metadataFlag {
			color.Magenta("Signature Detected in Metadata: " + name)
			foundFlag = true
		}
		if watermarkFlag {
			color.Magenta("Watermark Matched: " + name)
			foundFlag = true
		}
	}
	if !foundFlag {
		fmt.Println("No match found.")
	}
}

func getHash(file string) string {
	f, err := os.Open(file)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}

func applySignature(file string, signature string, binaryFlag bool, metadataFlag bool, watermarkFlag bool) {
	extension := filepath.Ext(file)
	if extension == ".pdf" && watermarkFlag {
		addWatermarkPDF(file, signature)
	}
	if metadataFlag {
		addMetadataSignature(file, signature)
	}
	if extension != ".docx" && extension != ".xlsx" && extension != ".pptx" {
		if binaryFlag {
			appendSignature(file, signature)
		}
	}

}

func appendSignature(file string, signature string) {
	signature = " " + signature
	f, err := os.OpenFile(file, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	defer f.Close()
	_, err = f.WriteString(signature)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func listFiles(root string) ([]string, error) {
	var files []string

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if !info.IsDir() {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return files, nil

}

func addMetadataSignature(file string, signature string) {
	var metaSection string
	extension := filepath.Ext(file)
	if extension == ".docx" || extension == ".xlsx" || extension == ".pptx" {
		workingDir, _ := filepath.Abs(filepath.Dir(file))
		Unzip(file, filepath.Join(workingDir, "temp"))
		coreFile := filepath.Join(workingDir, "temp", "docProps", "core.xml")
		if _, err := os.Stat(coreFile); os.IsNotExist(err) {
			color.Red("Document doesn't contain core.xml metadata section. It usually happens if you use Google Docs. Try to use Microsoft Office instead.")
			os.Exit(1)
		} else {
			f, err := os.Open(coreFile)
			if err != nil {
				color.Red("Error occurred during reading core.xml file")
				fmt.Println(err)
				os.Exit(1)
			}
			defer f.Close()
			scanner := bufio.NewScanner(f)
			var fileContent string
			for scanner.Scan() {
				fileContent += scanner.Text()
			}
			re := regexp.MustCompile(`<dc:creator>([\w\s]+)</dc:creator>`)
			match := re.FindStringSubmatch(fileContent)
			if match == nil {
				color.Red("Document doesn't contain core.xml metadata section. It usually happens if you use Google Docs. Try to use Microsoft Office instead.")
				os.Exit(1)
			}
			newCreator := strings.Replace(fileContent, match[0], "<dc:creator>"+signature+"</dc:creator>", 1)
			_ = ioutil.WriteFile(coreFile, []byte(newCreator), 0644)
			os.Remove(file)
			officeCompress(filepath.Join(workingDir, "temp"), file)
			os.RemoveAll(filepath.Join(workingDir, "temp"))
		}

	} else {
		switch {
		case extension == ".pdf":
			metaSection = "Producer"
		case extension == ".mov":
			metaSection = "Software"
		default:
			metaSection = "Title"
		}
		targetDir := filepath.Dir(file)
		tempDir := filepath.Join(targetDir, "temp")
		os.Mkdir(filepath.Join(tempDir), 0777)
		fileName := filepath.Base(file)
		tempFile := filepath.Join(tempDir, fileName)
		copyFile(file, tempFile)
		e, _ := exiftool.NewExiftool()
		defer e.Close()
		originals := e.ExtractMetadata(tempFile)
		originals[0].SetString(metaSection, signature)
		e.WriteMetadata(originals)
		os.Remove(file)
		copyFile(tempFile, file)
		os.RemoveAll(tempDir)

	}
}

func officeCompress(source string, target string) {
	files, err := listFiles(source)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	buf := new(bytes.Buffer)
	w := zip.NewWriter(buf)
	for _, file := range files {
		absPath, _ := filepath.Abs(file)
		relPath, _ := filepath.Rel(source, absPath)
		f, err := w.Create(relPath)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		fh, err := os.Open(absPath)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)

		}
		_, err = io.Copy(f, fh)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		fh.Close()
	}
	err = w.Close()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	err = ioutil.WriteFile(target, buf.Bytes(), 0644)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func exifRead(file string, field string) string {
	et, err := exiftool.NewExiftool()
	if err != nil {
		fmt.Printf("Error when intializing: %v\n", err)
		return ""
	}
	defer et.Close()

	fileInfos := et.ExtractMetadata(file)

	for _, fileInfo := range fileInfos {
		if fileInfo.Err != nil {
			fmt.Printf("Error concerning %v: %v\n", fileInfo.File, fileInfo.Err)
			continue
		}

		for k, v := range fileInfo.Fields {
			if k == field {
				exifValue := fmt.Sprint(v)
				return exifValue

			}
		}
	}
	return os.DevNull
}

func copyFile(src, dest string) (err error) {
	s, err := os.Open(src)
	if err != nil {
		return err
	}
	defer s.Close()

	d, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer d.Close()

	_, err = io.Copy(d, s)
	if err != nil {
		return err
	}
	return nil
}

func detectSignature(file string, signature string, hashValue string) (bool, bool, bool, bool) {
	binaryFlag := false
	hashFlag := false
	metadataFlag := false
	watermarkFlag := false
	fileHash := getHash(file)
	if fileHash == hashValue {
		hashFlag = true
	}
	var metaSection string
	f, err := os.Open(file)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	defer f.Close()
	var lines []string
	scanner := bufio.NewScanner(f)
	const maxCapacity = 5048 * 5048
	buf := make([]byte, maxCapacity)
	scanner.Buffer(buf, maxCapacity)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	for _, line := range lines {

		if strings.Contains(line, signature) {

			binaryFlag = true
		}
	}
	extension := filepath.Ext(file)
	switch {
	case extension == ".pdf":
		metaSection = "Producer"
		watermarkFlag = detectWatermarkPDF(file, signature)
	case extension == ".mov":
		metaSection = "Software"
	case extension == ".docx":
		metaSection = "Creator"
	case extension == ".xlsx":
		metaSection = "Creator"
	case extension == ".pptx":
		metaSection = "Creator"
	default:
		metaSection = "Title"
	}
	exifValue := exifRead(file, metaSection)
	metadataFlag = strings.Contains(exifValue, signature)

	return binaryFlag, hashFlag, metadataFlag, watermarkFlag
}

func readTargets(file string) []string {
	f, err := os.Open(file)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	defer f.Close()
	var lines []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	return lines
}

func generateSignature() string {
	id := uuid.New()
	return "75746b7573656e-" + id.String()
}

func generateTargetDB(file string, targets []string) {
	f, err := os.Create(file)
	if err != nil {
		color.Red("Can't create the database file")
		fmt.Println(err)
		os.Exit(1)
	}
	defer f.Close()
	for _, target := range targets {
		if target != "" {
			if !strings.Contains(target, ",") || strings.Count(target, ",") > 1 {
				color.Red("Wrong target format: " + target)
				color.Red("It should be something like this: Utku Sen,utku@utkusen.com")
				os.Exit(1)
			}
			_, err := f.WriteString(target + "," + generateSignature() + "\n")
			if err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
		}
	}
}

func createLocalFiles(baseFile string, projectName string, binaryFlag bool, metadataFlag bool, watermarkFlag bool) {
	currentDir, _ := os.Getwd()
	projectDir := filepath.Join(currentDir, projectName)
	fileDir := filepath.Join(projectDir, "files")
	targets := readTargets(filepath.Join(projectDir, "db.csv"))
	err := os.Mkdir(fileDir, 0777)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	var updatedDB string
	var fileHash string
	for _, target := range targets {
		if target != "" {
			signature := strings.Split(target, ",")[2]
			name := strings.Replace(strings.Split(target, ",")[0], " ", "_", -1)
			privateDir := filepath.Join(fileDir, name)
			err := os.Mkdir(privateDir, 0777)
			if err != nil {
				color.Red("Can't create the folder")
				fmt.Println(err)
				os.Exit(1)
			}
			fileLocation := filepath.Join(privateDir, filepath.Base(baseFile))
			_ = CopyFile(baseFile, fileLocation)
			applySignature(fileLocation, signature, binaryFlag, metadataFlag, watermarkFlag)
			fileHash = getHash(fileLocation)
			updatedDB += target + "," + fileHash + "," + fileLocation + "\n"

		}
		err = ioutil.WriteFile(filepath.Join(projectDir, "db.csv"), []byte(updatedDB), 0644)
		if err != nil {
			color.Red("Can't write to the database")
			fmt.Println(err)
			os.Exit(1)
		}
	}
}

func CopyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	cerr := out.Close()
	if err != nil {
		return err
	}
	return cerr
}

func Unzip(src string, destination string) ([]string, error) {
	var filenames []string
	r, err := zip.OpenReader(src)
	if err != nil {
		return filenames, err
	}

	defer r.Close()
	for _, f := range r.File {
		fpath := filepath.Join(destination, f.Name)
		if !strings.HasPrefix(fpath, filepath.Clean(destination)+string(os.PathSeparator)) {
			return filenames, fmt.Errorf("%s is an illegal filepath", fpath)
		}
		filenames = append(filenames, fpath)

		if f.FileInfo().IsDir() {
			os.MkdirAll(fpath, os.ModePerm)
			continue
		}

		if err = os.MkdirAll(filepath.Dir(fpath), os.ModePerm); err != nil {
			return filenames, err
		}
		outFile, err := os.OpenFile(fpath,
			os.O_WRONLY|os.O_CREATE|os.O_TRUNC,
			f.Mode())
		if err != nil {
			return filenames, err
		}

		rc, err := f.Open()
		if err != nil {
			return filenames, err
		}

		_, err = io.Copy(outFile, rc)
		outFile.Close()
		rc.Close()
		if err != nil {
			return filenames, err
		}
	}
	return filenames, nil
}

func GetFileContentType(out *os.File) string {
	buffer := make([]byte, 512)
	_, err := out.Read(buffer)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	contentType := http.DetectContentType(buffer)

	return contentType
}

func sendWithSendgrid(toName string, toEmail string, fromName string, fromEmail string, subject string, bodyFile string, contentType string, attachment string) {
	configFile, err := os.Open("CONFIG")
	if err != nil {
		color.Red("Can't read the CONFIG file")
		fmt.Println(err)
		os.Exit(1)
	}
	defer configFile.Close()
	scanner := bufio.NewScanner(configFile)
	for scanner.Scan() {
		if strings.Contains(scanner.Text(), "SENDGRID_API_KEY") {
			apiKey := strings.TrimSpace(strings.Replace(strings.Split(scanner.Text(), "=")[1], "\"", "", -1))
			if apiKey == "" {
				fmt.Println("No Sendgrid API Key is set in the CONFIG file")
				os.Exit(1)
			} else {
				body, err := ioutil.ReadFile(bodyFile)
				if err != nil {
					color.Red("Error occurred while reading the template file")
					fmt.Println(err)
					os.Exit(1)
				}
				m := mail.NewV3Mail()
				newBody := strings.Replace(string(body), "{{Name}}", toName, -1)
				from := mail.NewEmail(fromName, fromEmail)
				to := mail.NewEmail(toName, toEmail)
				content := mail.NewContent("text/plain", newBody)
				if contentType == "text" {
					content = mail.NewContent("text/plain", newBody)
				} else if contentType == "html" {
					content = mail.NewContent("text/html", newBody)
				} else {
					fmt.Println("Content type is not set correctly")
					os.Exit(1)
				}
				m.SetFrom(from)
				m.AddContent(content)
				personalization := mail.NewPersonalization()
				personalization.AddTos(to)
				personalization.Subject = subject
				m.AddPersonalizations(personalization)
				f, err := os.Open(attachment)
				if err != nil {
					color.Red("Can't open the attachment file")
					fmt.Println(err)
					os.Exit(1)
				}
				defer f.Close()
				contentType := GetFileContentType(f)
				attachmentFile := mail.NewAttachment()
				dat, err := ioutil.ReadFile(attachment)
				if err != nil {
					fmt.Println(err)
				}
				encoded := base64.StdEncoding.EncodeToString([]byte(dat))
				attachmentFile.SetContent(encoded)
				attachmentFile.SetType(contentType)
				fileName := filepath.Base(attachment)
				attachmentFile.SetFilename(fileName)
				attachmentFile.SetDisposition("attachment")
				m.AddAttachment(attachmentFile)
				request := sendgrid.GetRequest(apiKey, "/v3/mail/send", "https://api.sendgrid.com")
				request.Method = "POST"
				request.Body = mail.GetRequestBody(m)
				response, err := sendgrid.API(request)
				if err != nil {
					color.Red("Error occurred while using Sendgrid's API")
					fmt.Println(err)
					os.Exit(1)
				}
				if response.StatusCode == 202 {
					color.Green("Email sent successfully to : " + toName + " (" + toEmail + ")")
				} else {
					color.Red("Email couldn't sent to : " + toName + " (" + toEmail + ") Check your Sendgrid Integration:")
					fmt.Println(response)
					os.Exit(1)
				}

			}

		}
	}
}

func sendWithSES(toName string, toEmail string, fromName string, fromEmail string, subject string, bodyFile string, contentType string, attachment string, region string) {
	sess, err := session.NewSession(&aws.Config{
		Region: aws.String(region)},
	)
	if err != nil {
		color.Red("Error occurred while creating AWS session")
		fmt.Println(err)
		os.Exit(1)
	}
	body, err := ioutil.ReadFile(bodyFile)
	if err != nil {
		color.Red("Error occurred while reading the template file")
		fmt.Println(err)
		os.Exit(1)
	}
	newBody := strings.Replace(string(body), "{{Name}}", toName, -1)
	msg := gomail.NewMessage()
	svc := ses.New(sess)
	msg.SetAddressHeader("From", fromEmail, fromName)
	type Recipient struct {
		toEmails []string
	}
	recipient := Recipient{
		toEmails: []string{toEmail},
	}
	var recipients []*string
	for _, r := range recipient.toEmails {
		recipient := r
		recipients = append(recipients, &recipient)
	}
	msg.SetHeader("To", recipient.toEmails...)
	msg.SetHeader("Subject", subject)
	if contentType == "text" {
		msg.SetBody("text/plain", newBody)
	} else if contentType == "html" {
		msg.SetBody("text/html", newBody)
	} else {
		fmt.Println("Content type is not set correctly")
		os.Exit(1)
	}
	msg.Attach(attachment)
	var emailRaw bytes.Buffer
	msg.WriteTo(&emailRaw)
	message := ses.RawMessage{Data: emailRaw.Bytes()}
	input := &ses.SendRawEmailInput{Source: &fromEmail, Destinations: recipients, RawMessage: &message}
	_, err = svc.SendRawEmail(input)
	if err != nil {
		color.Red("Email couldn't sent to : " + toName + " (" + toEmail + ")")
		fmt.Println(err)
		os.Exit(1)
	}
	color.Green("Email sent successfully to : " + toName + " (" + toEmail + ")")
}

func sendWithSMTP(toName string, toEmail string, fromName string, fromEmail string, subject string, bodyFile string, contentType string, attachment string) {
	configs := parseConfigFile()

	if configs["SMTP_SERVER"] == "" {
		color.Red("No SMTP Server is set in the CONFIG file")
		os.Exit(1)
	}

	if configs["SMTP_PORT"] == "" {
		color.Red("No SMTP Port is set in the CONFIG file")
		os.Exit(1)
	}

	m := gomail.NewMessage()
	m.SetHeader("From", m.FormatAddress(fromEmail, fromName))
	m.SetHeader("To", toEmail)
	m.SetHeader("Subject", subject)
	body, err := ioutil.ReadFile(bodyFile)
	if err != nil {
		color.Red("Error occurred while reading the template file")
		fmt.Println(err)
		os.Exit(1)
	}
	newBody := strings.Replace(string(body), "{{Name}}", toName, -1)
	if contentType == "text" {
		m.SetBody("text/plain", newBody)
	} else if contentType == "html" {
		m.SetBody("text/html", newBody)
	} else {
		color.Red("Content type is not set correctly")
		os.Exit(1)
	}
	m.Attach(attachment)
	intPort, err := strconv.Atoi(configs["SMTP_PORT"])
	if err != nil {
		color.Red("Invalid SMTP Port")
		os.Exit(1)
	}
	d := gomail.NewDialer(configs["SMTP_SERVER"], intPort, configs["SMTP_USERNAME"], configs["SMTP_PASSWORD"])
	d.TLSConfig = &tls.Config{InsecureSkipVerify: true}
	if err := d.DialAndSend(m); err != nil {
		color.Red("Email couldn't sent to : " + toName + " (" + toEmail + ")")
		fmt.Println(err)
		os.Exit(1)
	} else {
		color.Green("Email sent successfully to : " + toName + " (" + toEmail + ")")
	}

}

func parseConfigFile() map[string]string {
	f, err := os.Open("CONFIG")
	if err != nil {
		color.Red("Can't read the CONFIG file")
		fmt.Println(err)
		os.Exit(1)
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	var config = make(map[string]string)
	for scanner.Scan() {
		if strings.Contains(scanner.Text(), "=") {
			key := strings.TrimSpace(strings.Split(scanner.Text(), "=")[0])
			value := strings.TrimSpace(strings.Replace(strings.Split(scanner.Text(), "=")[1], "\"", "", -1))
			config[key] = value
		}
	}
	return config
}
