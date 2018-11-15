package main

import (
	"encoding/json"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/fe0b6/tools"

	"github.com/fe0b6/sigwaiter"
)

const authKey = ""

var (
	exited bool
	wg     sync.WaitGroup
	tmpDir string
)

type optiConf struct {
	WSizes      []int
	Formats     []string
	WebpQuality string
}

type fileData struct {
	Data   []byte
	Format string
	Size   string
}

func init() {
	tmpDir = "/tmp/"

	// Добавляем дату, время и строку в какой идет запись в лог
	log.SetFlags(log.Ldate | log.Ltime | log.Llongfile)
}

func main() {
	rand.Seed(time.Now().UnixNano())

	go func() {
		http.HandleFunc("/", parseRequest)

		log.Fatalln("[fatal]", http.ListenAndServe(":8080", nil))
	}()

	wchan := make(chan bool)
	go waitExit(wchan)

	// Перехватываем сигналы
	sigwaiter.Run(10, wchan)
}

// Ждем сигнал о выходе
func waitExit(exitChan chan bool) {
	_ = <-exitChan
	exited = true

	log.Println("[info]", "Завершаем работу")
	// Ждем пока все запросы завершатся
	wg.Wait()
	log.Println("[info]", "Работа сервера завершена корректно")
	exitChan <- true
}

// Разбираем запрос
func parseRequest(w http.ResponseWriter, r *http.Request) {

	// Если сервер завершает работу
	if exited {
		w.WriteHeader(503)
		w.Write([]byte(http.StatusText(503)))
		return
	}

	// Отмечаем что начался новый запрос
	wg.Add(1)

	// По завершению запроса отмечаем что он закончился
	defer wg.Done()

	// Парсим тело запроса
	err := r.ParseMultipartForm(1024 * 1024 * 1024 * 100)
	if err != nil {
		log.Println("[error]", err)
		w.WriteHeader(500)
		w.Write([]byte(err.Error()))
		return
	}

	// Проверяем авторизацию
	if authKey != r.FormValue("key") {
		log.Println("[error]", "try unauth access")
		w.WriteHeader(403)
		w.Write([]byte(http.StatusText(403)))
		return
	}

	// читаем файл
	file, _, err := r.FormFile("image")
	if err != nil {
		log.Println("[error]", err)
		w.WriteHeader(500)
		w.Write([]byte(err.Error()))
		return
	}
	defer file.Close()
	//log.Println(h.Filename, h.Size, h.Header)

	// Проверяем формат
	_, format, err := image.Decode(file)
	if err != nil {
		log.Println("[error]", err)
		w.WriteHeader(500)
		w.Write([]byte(err.Error()))
		return
	}

	// Формируем имя временного файла
	path := tmpDir + strconv.FormatInt(time.Now().UnixNano(), 32) + "_" + strconv.FormatInt(rand.Int63(), 32) + "/"
	err = os.Mkdir(path, 0700)
	if err != nil {
		log.Println("[error]", err)
		w.WriteHeader(500)
		w.Write([]byte(err.Error()))
		return
	}
	defer os.RemoveAll(path)

	fname := "orig." + format

	// Пишем временный файл
	out, err := os.Create(path + fname)
	if err != nil {
		log.Println("[error]", err)
		w.WriteHeader(500)
		w.Write([]byte(err.Error()))
		return
	}
	defer out.Close()

	// Возвращаем указатель в начала данныъ
	_, err = file.Seek(0, 0)
	if err != nil {
		log.Println("[error]", err)
		w.WriteHeader(500)
		w.Write([]byte(err.Error()))
		return
	}

	// Копируем данные
	_, err = io.Copy(out, file)
	if err != nil {
		log.Println("[error]", err)
		w.WriteHeader(500)
		w.Write([]byte(err.Error()))
		return
	}

	// Закроем файл
	out.Close()

	optiC := optiConf{
		WebpQuality: r.FormValue("webp_quality"),
	}
	if optiC.WebpQuality == "" {
		optiC.WebpQuality = "90"
	}

	if r.FormValue("wsizes") != "" {
		err = json.Unmarshal([]byte(r.FormValue("wsizes")), &optiC.WSizes)
		if err != nil {
			log.Println("[error]", err)
			w.WriteHeader(500)
			w.Write([]byte(err.Error()))
			return
		}
	}

	if r.FormValue("formats") != "" {
		err = json.Unmarshal([]byte(r.FormValue("formats")), &optiC.Formats)
		if err != nil {
			log.Println("[error]", err)
			w.WriteHeader(500)
			w.Write([]byte(err.Error()))
			return
		}
	}

	// Сначала проводим оптимизации
	switch format {
	case "jpeg":
		cmd := exec.Command("/usr/bin/jpegoptim", "--strip-all", path+fname)
		_, err := cmd.Output()
		if err != nil {
			log.Println("[error]", err)
			w.WriteHeader(500)
			w.Write([]byte(err.Error()))
			return
		}
	case "png":
		cmd := exec.Command("/usr/bin/optipng", path+fname)
		_, err := cmd.Output()
		if err != nil {
			log.Println("[error]", err)
			w.WriteHeader(500)
			w.Write([]byte(err.Error()))
			return
		}
	}

	fnames := []string{"orig"}
	// Если надо отресайзить
	if len(optiC.WSizes) > 0 {
		for _, size := range optiC.WSizes {
			sizeName := fmt.Sprintf("%d.%s", size, format)
			fnames = append(fnames, fmt.Sprintf("%d", size))

			cmd := exec.Command("/usr/bin/convert", "-resize", fmt.Sprintf("%dx", size), path+fname, path+sizeName)
			_, err := cmd.Output()
			if err != nil {
				log.Println("[error]", err)
				w.WriteHeader(500)
				w.Write([]byte(err.Error()))
				return
			}
		}
	}

	// Проверяем нужны ли другие форматы
	if len(optiC.Formats) > 0 {
		for _, fr := range optiC.Formats {
			if fr == format {
				continue
			}
			for _, fn := range fnames {

				var cmd *exec.Cmd
				switch fr {
				case "webp":
					cmd = exec.Command("/usr/bin/convert", "-quality", optiC.WebpQuality, path+fn+"."+format, path+fn+"."+fr)
				default:
					cmd = exec.Command("/usr/bin/convert", path+fn+"."+format, path+fn+"."+fr)
				}

				b, err := cmd.CombinedOutput()
				if err != nil {
					log.Println("[error]", string(b), err)
					w.WriteHeader(500)
					w.Write([]byte(err.Error()))
					return
				}
			}
		}
	}

	files, err := ioutil.ReadDir(path)
	if err != nil {
		log.Println("[error]", err)
		w.WriteHeader(500)
		w.Write([]byte(err.Error()))
		return
	}

	ans := []fileData{}
	for _, f := range files {
		b, err := ioutil.ReadFile(path + f.Name())
		if err != nil {
			log.Println("[error]", err)
			w.WriteHeader(500)
			w.Write([]byte(err.Error()))
			return
		}

		d := strings.Split(f.Name(), ".")

		fd := fileData{
			Size:   d[0],
			Format: d[1],
			Data:   b,
		}

		ans = append(ans, fd)
	}

	w.Write(tools.ToGob(ans))
}
