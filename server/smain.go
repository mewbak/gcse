/*
	GCSE HTTP server.
*/
package main

import (
	"compress/gzip"
	"fmt"
	godoc "go/doc"
	"html/template"
	"log"
	"net/http"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/golangplus/bytes"
	"github.com/golangplus/strings"
	"github.com/golangplus/time"
	"golang.org/x/net/trace"

	"github.com/ajstarks/svgo"
	"github.com/daviddengcn/gcse"
	"github.com/daviddengcn/gcse/configs"
	"github.com/daviddengcn/gddo/doc"
	"github.com/daviddengcn/go-easybi"
	"github.com/daviddengcn/go-index"
	"github.com/russross/blackfriday"
)

type UIUtils struct{}

func (UIUtils) Slice(els ...interface{}) interface{} {
	return append([]interface{}(nil), els...)
}

func (UIUtils) Add(vl, delta int) int {
	return vl + delta
}

var templates *template.Template

func Markdown(templ string) template.HTML {
	var out bytesp.Slice
	templates.ExecuteTemplate(&out, templ, nil)
	return template.HTML(blackfriday.MarkdownCommon(out))
}

func loadTemplates() {
	templates = template.Must(template.New("templates").Funcs(template.FuncMap{
		"markdown": Markdown,
	}).ParseGlob(configs.ServerRoot.Join(`web/*`).S()))
}

func reloadTemplates() {
	if configs.AutoLoadTemplate {
		loadTemplates()
	}
}

func init() {
	log.SetFlags(log.Flags() | log.Lmicroseconds)

	http.Handle("/css/", http.StripPrefix("/css/", http.FileServer(http.Dir(configs.ServerRoot.Join("css").S()))))
	http.Handle("/js/", http.StripPrefix("/js/", http.FileServer(http.Dir(configs.ServerRoot.Join("js").S()))))
	http.Handle("/images/", http.StripPrefix("/images/", http.FileServer(http.Dir(configs.ServerRoot.Join("images").S()))))
	http.Handle("/img/", http.StripPrefix("/img/", http.FileServer(http.Dir(configs.ServerRoot.Join("images").S()))))
	http.Handle("/robots.txt", http.FileServer(http.Dir(configs.ServerRoot.Join("static").S())))
	http.Handle("/clippy.swf", http.FileServer(http.Dir(configs.ServerRoot.Join("static").S())))

	http.HandleFunc("/add", pageAdd)
	http.HandleFunc("/search", pageSearch)
	http.HandleFunc("/view", pageView)
	http.HandleFunc("/tops", pageTops)
	http.HandleFunc("/about", staticPage("about.html"))
	http.HandleFunc("/infoapi", staticPage("infoapi.html"))
	http.HandleFunc("/api", pageApi)
	http.HandleFunc("/loadtemplates", pageLoadTemplate)
	http.HandleFunc("/badge", pageBadge)
	http.HandleFunc("/badgepage", pageBadgePage)
	bi.HandleRequest(configs.BiWebPath)

	http.HandleFunc("/", pageRoot)
}

func pageLoadTemplate(w http.ResponseWriter, r *http.Request) {
	if configs.LoadTemplatePass != "" {
		pass := r.FormValue("pass")
		if pass != configs.LoadTemplatePass {
			w.Write([]byte("Incorrect password!"))
			return
		}
	}
	loadTemplates()
	w.Write([]byte("Tempates loaded."))
}

type globalHandler struct{}

type gzipResponseWriter struct {
	http.ResponseWriter
	gzipWriter *gzip.Writer
}

func (gzr gzipResponseWriter) Write(bs []byte) (int, error) {
	return gzr.gzipWriter.Write(bs)
}

func (hdl globalHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	reloadTemplates()

	log.Printf("[B] %s %v %s %v %v %v", r.Method, r.RequestURI, r.RemoteAddr, r.Header.Get("X-Forwarded-For"), r.Header.Get("Referer"), r.Header.Get("User-Agent"))
	if strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
		w.Header().Set("Content-Encoding", "gzip")
		gzr := gzipResponseWriter{
			ResponseWriter: w,
			gzipWriter:     gzip.NewWriter(w),
		}
		defer gzr.gzipWriter.Close()
		http.DefaultServeMux.ServeHTTP(gzr, r)
	} else {
		http.DefaultServeMux.ServeHTTP(w, r)
	}
	log.Printf("[E] %s %v %s %v %v %v", r.Method, r.RequestURI, r.RemoteAddr, r.Header.Get("X-Forwarded-For"), r.Header.Get("Referer"), r.Header.Get("User-Agent"))
}

func processBi() {
	for {
		bi.Process()
		time.Sleep(time.Minute)
	}
}

func main() {
	runtime.GOMAXPROCS(2)
	if err := gcse.ImportSegments.ClearUndones(); err != nil {
		log.Printf("CleanImportSegments failed: %v", err)
	}
	if err := loadIndex(); err != nil {
		log.Fatal(err)
	}
	go loadIndexLoop()
	go processBi()

	loadTemplates()

	log.Printf("ListenAndServe at %s ...", configs.ServerAddr)

	log.Fatal(http.ListenAndServe(configs.ServerAddr, globalHandler{}))
}

type SimpleDuration time.Duration

func (sd SimpleDuration) String() string {
	d := time.Duration(sd)
	if d > timep.Day {
		return fmt.Sprintf("%.0f days", d.Hours()/24)
	}
	if d >= time.Hour {
		return fmt.Sprintf("%.0f hours", d.Hours())
	}
	if d >= time.Minute {
		return fmt.Sprintf("%.0f mins", d.Minutes())
	}
	if d >= time.Second {
		return fmt.Sprintf("%.0f sec", d.Seconds())
	}
	if d >= time.Millisecond {
		return fmt.Sprintf("%d ms", d.Nanoseconds()/1e6)
	}
	if d >= time.Microsecond {
		return fmt.Sprintf("%d us", d.Nanoseconds()/1e3)
	}
	return fmt.Sprintf("%d ns", d.Nanoseconds())
}

func pageRoot(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	if r.URL.Path != "/" {
		w.WriteHeader(http.StatusNotFound)
		if err := templates.ExecuteTemplate(w, "404.html", struct {
			UIUtils
			Path string
		}{
			Path: r.URL.Path,
		}); err != nil {
			w.Write([]byte(err.Error()))
		}
		return
	}
	db := getDatabase()
	if err := templates.ExecuteTemplate(w, "index.html", struct {
		UIUtils
		TotalDocs     int
		TotalProjects int
		LastUpdated   time.Time
		IndexAge      SimpleDuration
	}{
		TotalDocs:     db.PackageCount(),
		TotalProjects: db.ProjectCount(),
		LastUpdated:   db.IndexUpdated(),
		IndexAge:      SimpleDuration(time.Since(db.IndexUpdated())),
	}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func staticPage(tempName string) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")

		if err := templates.ExecuteTemplate(w, tempName, struct {
			UIUtils
		}{}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
}

func filterPackages(pkgs []string) (res []string) {
	for _, pkg := range pkgs {
		pkg = gcse.TrimPackageName(pkg)
		if !doc.IsValidRemotePath(pkg) {
			continue
		}
		res = append(res, pkg)
	}
	return
}

func pageAdd(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")

	pkgsStr := r.FormValue("pkg")
	pkgMessage := ""
	msgCls := "success"
	taValue := ""
	if pkgsStr != "" {
		pkgs := filterPackages(strings.Split(pkgsStr, "\n"))
		if len(pkgs) > 0 {
			log.Printf("%d packages added!", len(pkgs))
			pkgMessage = fmt.Sprintf("Totally %d package(s) added!", len(pkgs))
			gcse.AppendPackages(pkgs)
		} else {
			msgCls = "danger"
			pkgMessage = "No package added! Check the format you submitted, please."
			taValue = pkgsStr
		}
	}
	err := templates.ExecuteTemplate(w, "add.html", struct {
		UIUtils
		Message string
		MsgCls  string
		TAValue string
	}{
		Message: pkgMessage,
		MsgCls:  msgCls,
		TAValue: taValue,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

type SubProjectInfo struct {
	MarkedName template.HTML
	Package    string
	SubPath    string
	Info       string
}

type ShowDocInfo struct {
	*Hit
	Index         int
	Summary       template.HTML
	MarkedName    template.HTML
	MarkedPackage template.HTML
	Subs          []SubProjectInfo
}

type ShowResults struct {
	TotalResults int
	TotalEntries int
	Folded       int
	Docs         []ShowDocInfo
}

func markWord(word []byte) []byte {
	buf := bytesp.Slice("<b>")
	template.HTMLEscape(&buf, word)
	buf.Write([]byte("</b>"))
	return buf
}

func markText(text string, tokens stringsp.Set, markFunc func([]byte) []byte) template.HTML {
	if len(text) == 0 {
		return ""
	}
	var outBuf bytesp.Slice

	index.MarkText([]byte(text), gcse.CheckRuneType, func(token []byte) bool {
		// needMark
		return tokens.Contain(gcse.NormWord(string(token)))
	}, func(text []byte) error {
		// output
		template.HTMLEscape(&outBuf, text)
		return nil
	}, func(token []byte) error {
		outBuf.Write(markFunc(token))
		return nil
	})
	return template.HTML(string(outBuf))
}

type Range struct {
	start, count int
}

func (r Range) In(idx int) bool {
	return idx >= r.start && idx < r.start+r.count
}

func packageShowName(name, pkg string) string {
	if name != "" && name != "main" {
		return name
	}
	prj := gcse.ProjectOfPackage(pkg)

	if name == "main" {
		return "main - " + prj
	}
	return "(" + prj + ")"
}

func showSearchResults(db database, results *SearchResult, tokens stringsp.Set, r Range) *ShowResults {
	docs := make([]ShowDocInfo, 0, len(results.Hits))

	projToIdx := make(map[string]int)
	folded := 0

	cnt := 0
mainLoop:
	for _, d := range results.Hits {
		d.Name = packageShowName(d.Name, d.Package)

		parts := strings.Split(d.Package, "/")
		if len(parts) > 2 {
			// try fold it (if its parent has been in the list)
			for i := len(parts) - 1; i >= 2; i-- {
				pkg := strings.Join(parts[:i], "/")
				if idx, ok := projToIdx[pkg]; ok {
					markedName := markText(d.Name, tokens, markWord)
					if r.In(idx) {
						docsIdx := idx - r.start
						docs[docsIdx].Subs = append(docs[docsIdx].Subs,
							SubProjectInfo{
								MarkedName: markedName,
								Package:    d.Package,
								SubPath:    "/" + strings.Join(parts[i:], "/"),
								Info:       d.Synopsis,
							})
					}
					folded++
					continue mainLoop
				}
			}
		}
		projToIdx[d.Package] = cnt
		if r.In(cnt) {
			markedName := markText(d.Name, tokens, markWord)
			readme := ""
			desc := d.Description
			if hit, found := db.FindFullPackage(d.Package); found {
				readme := gcse.ReadmeToText(d.ReadmeFn, d.ReadmeData)
				if len(readme) > 20*1024 {
					readme = readme[:20*1024]
				}
				desc = hit.Description
			}
			for _, sent := range d.ImportantSentences {
				desc += "\n" + sent
			}
			desc += "\n" + readme
			raw := selectSnippets(desc, tokens, 300)

			if d.StarCount < 0 {
				d.StarCount = 0
			}
			docs = append(docs, ShowDocInfo{
				Hit:           d,
				Index:         cnt + 1,
				MarkedName:    markedName,
				Summary:       markText(raw, tokens, markWord),
				MarkedPackage: markText(d.Package, tokens, markWord),
			})
		}
		cnt++
	}
	return &ShowResults{
		TotalResults: results.TotalResults,
		TotalEntries: cnt,
		Folded:       folded,
		Docs:         docs,
	}
}

const itemsPerPage = 10

func pageSearch(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")

	tr := trace.New("pageSearch", r.URL.Path)
	defer tr.Finish()

	// current page, 1-based
	p, err := strconv.Atoi(r.FormValue("p"))
	if err != nil {
		p = 1
	}
	startTime := time.Now()

	q := strings.TrimSpace(r.FormValue("q"))
	db := getDatabase()
	results, tokens, err := search(tr, db, q)
	if err != nil {
		tr.LazyPrintf("search failed: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	tr.LazyPrintf("Search success with %d hits and %d tokens", len(results.Hits), len(tokens))
	showResults := showSearchResults(db, results, tokens, Range{(p - 1) * itemsPerPage, itemsPerPage})
	tr.LazyPrintf("showSearchResults with %d results", len(showResults.Docs))
	totalPages := (showResults.TotalEntries + itemsPerPage - 1) / itemsPerPage
	log.Printf("totalPages: %d", totalPages)
	var beforePages, afterPages []int
	for i := 1; i <= totalPages; i++ {
		if i < p && p-i < 10 {
			beforePages = append(beforePages, i)
		} else if i > p && i-p < 10 {
			afterPages = append(afterPages, i)
		}
	}
	prevPage, nextPage := p-1, p+1
	if prevPage < 0 || prevPage > totalPages {
		prevPage = 0
	}
	if nextPage < 0 || nextPage > totalPages {
		nextPage = 0
	}
	searchDue := time.Since(startTime)
	if searchDue <= time.Second {
		bi.AddValue(bi.Sum, "search.latency.<=1s", 1)
	} else {
		bi.AddValue(bi.Sum, "search.latency.>1", 1)
		if searchDue > 10*time.Second {
			bi.AddValue(bi.Sum, "search.latency.>10", 1)
			if searchDue > 100*time.Second {
				bi.AddValue(bi.Sum, "search.latency.>100s", 1)
			}
		}
	}
	data := struct {
		UIUtils
		Q           string
		Results     *ShowResults
		SearchTime  SimpleDuration
		BeforePages []int
		PrevPage    int
		CurrentPage int
		NextPage    int
		AfterPages  []int
		BottomQ     bool
		TotalPages  int
	}{
		Q:           q,
		Results:     showResults,
		SearchTime:  SimpleDuration(searchDue),
		BeforePages: beforePages,
		PrevPage:    prevPage,
		CurrentPage: p,
		NextPage:    nextPage,
		AfterPages:  afterPages,
		BottomQ:     len(results.Hits) >= 5,
		TotalPages:  totalPages,
	}
	log.Printf("Search results ready")
	err = templates.ExecuteTemplate(w, "search.html", data)
	if err != nil {
		tr.LazyPrintf("ExecuteTemplate failed: %v", err)
		tr.SetError()
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
	log.Printf("Search results rendered")
}

func pageView(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")

	id := strings.TrimSpace(r.FormValue("id"))
	if id != "" {
		db := getDatabase()
		doc, found := db.FindFullPackage(id)
		if !found {
			http.Error(w, fmt.Sprintf("Package %s not found!", id), http.StatusNotFound)
			return
		}
		if doc.StarCount < 0 {
			doc.StarCount = 0
		}
		var descHTML bytesp.Slice
		godoc.ToHTML(&descHTML, doc.Description, nil)

		if err := templates.ExecuteTemplate(w, "view.html", struct {
			UIUtils
			gcse.HitInfo
			DescHTML      template.HTML
			TotalDocCount int
			StaticRank    int
			ShowReadme    bool
		}{
			HitInfo:       doc,
			DescHTML:      template.HTML(descHTML),
			TotalDocCount: db.PackageCount(),
			StaticRank:    doc.StaticRank + 1,
			ShowReadme:    len(doc.Description) < 10 && len(doc.ReadmeData) > 0,
		}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}
}

func pageTops(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")

	N, _ := strconv.Atoi(r.FormValue("len"))
	if N < 20 {
		N = 20
	} else if N > 100 {
		N = 100
	}
	if err := templates.ExecuteTemplate(w, "tops.html", struct {
		UIUtils
		Lists []StatList
	}{
		Lists: statTops(N),
	}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func pageBadgePage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	id := strings.TrimSpace(r.FormValue("id"))
	if id != "" {
		doc, found := getDatabase().FindFullPackage(id)
		if !found {
			http.Error(w, fmt.Sprintf("Package %s not found!", id), http.StatusNotFound)
			return
		}
		badgeUrl := "http://go-search.org/badge?id=" + template.URLQueryEscaper(doc.Package)
		viewUrl := "http://go-search.org/view?id=" + template.URLQueryEscaper(doc.Package)

		htmlCode := fmt.Sprintf(`<a href="%s"><img src="%s" alt="GoSearch"></a>`, viewUrl, badgeUrl)
		mdCode := fmt.Sprintf(`[![GoSearch](%s)](%s)`, badgeUrl, viewUrl)

		if err := templates.ExecuteTemplate(w, "badgepage.html", struct {
			UIUtils
			gcse.HitInfo
			HTMLCode string
			MDCode   string
		}{
			HitInfo:  doc,
			HTMLCode: htmlCode,
			MDCode:   mdCode,
		}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}
}

func pageBadge(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.FormValue("id"))
	if id != "" {
		doc, found := getDatabase().FindFullPackage(id)
		if !found {
			http.Error(w, fmt.Sprintf("Package %s not found!", id), http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "image/svg+xml")

		W, H := 100, 22

		s := svg.New(w)
		s.Start(W, H)
		s.Roundrect(1, 1, W-2, H-2, 4, 4, "fill:#5bc0de")

		s.Text(5, 15, fmt.Sprintf("GoSearch #%d", doc.StaticRank+1),
			`font-size:10;fill:white;font-weight:bold;font-family:Arial, Helvetica, sans-serif`)
		s.End()
	}
}
