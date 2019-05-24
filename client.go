package imgopti

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"mime/multipart"
	"net/http"

	"github.com/fe0b6/tools"
)

// FileData - объект ответа
type FileData struct {
	Data   []byte
	Format string
	Size   string
}

// ProcessImage - отправляем фотку на обработку
func ProcessImage(f io.Reader, endpoint string, params map[string][]byte) (fds []FileData, err error) {

	// Создаем буфер в который сложим запрос
	var b bytes.Buffer
	w := multipart.NewWriter(&b)
	defer w.Close()

	// Добавим файл
	fw, err := w.CreateFormFile("image", "image")
	if err != nil {
		log.Println("[error]", err)
		return
	}
	if _, err = io.Copy(fw, f); err != nil {
		log.Println("[error]", err)
		return
	}

	// Добавляем параметры
	for k, v := range params {
		fw, err = w.CreateFormField(k)
		if err != nil {
			log.Println("[error]", err)
			return
		}
		if _, err = fw.Write(v); err != nil {
			log.Println("[error]", err)
			return
		}
	}

	// закрываем multipart
	w.Close()

	// Now that you have a form, you can submit it to your handler.
	req, err := http.NewRequest("POST", endpoint, &b)
	if err != nil {
		log.Println("[error]", err)
		return
	}
	// Don't forget to set the content type, this will contain the boundary.
	req.Header.Set("Content-Type", w.FormDataContentType())

	// Submit the request
	client := &http.Client{}
	resp, err := client.Do(req)
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		log.Println("[error]", err)
		return
	}

	// Проверяем что ответ хороший
	if resp.StatusCode != 200 {
		log.Println("[error]", resp.StatusCode, resp.Status)
		return
	}

	// Читаем ответ
	content, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Println("[error]", err)
		return
	}

	// Парсим данные
	tools.FromGob(&fds, content)
	if len(fds) == 0 {
		err = fmt.Errorf("Пустой ответ")
		log.Println("[error]", err)
		return
	}

	return
}
