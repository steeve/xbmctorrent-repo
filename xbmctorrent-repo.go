package main

import (
    "os"
    "bufio"
    "bytes"
    "strings"
    "fmt"
    "net/http"
    "io/ioutil"
    "encoding/json"
    "encoding/xml"
    "crypto/md5"
    "github.com/gorilla/mux"
    "html/template"
)


var IndexTemplate, _ = template.New("index").Parse(`
<html>
    <head><title>Index</title></head>
    <body>
        <ul>
            {{ range . }}
            <li><a href="{{.}}">{{.}}</a></li>
            {{ end }}
        </ul>
    </body>
</html>
`)


// "url": "https://api.github.com/repos/steeve/xbmctorrent/releases/assets/33121",
// "id": 33121,
// "name": "plugin.video.xbmctorrent-0.4.1.zip",
// "label": "plugin.video.xbmctorrent-0.4.1.zip",
// "content_type": "application/zip",
// "state": "uploaded",
// "size": 24714142,
// "download_count": 356,
// "created_at": "2013-11-01T20:42:32Z",
// "updated_at": "2013-11-01T20:47:38Z"
type ReleaseAsset struct {
    URL                 string  `json:"url"`
    ID                  int64   `json:"id"`
    Name                string  `json:"name"`
    Label               string  `json:"label"`
    ContentType         string  `json:"content_type"`
    State               string  `json:"state"`
    Size                int64   `json:"size"`
    DownloadCount       int64   `json:"download_count"`
    CreatedAt           string  `json:"created_at"`
    UpdatedAt           string  `json:"published_at"`
}

// "url": "https://api.github.com/repos/octocat/Hello-World/releases/1",
// "html_url": "https://github.com/octocat/Hello-World/releases/v1.0.0",
// "assets_url": "https://api.github.com/repos/octocat/Hello-World/releases/1/assets",
// "upload_url": "https://uploads.github.com/repos/octocat/Hello-World/releases/1/assets{?name}",
// "id": 1,
// "tag_name": "v1.0.0",
// "target_commitish": "master",
// "name": "v1.0.0",
// "body": "Description of the release",
// "draft": false,
// "prerelease": false,
// "created_at": "2013-02-27T19:35:32Z",
// "published_at": "2013-02-27T19:35:32Z"
type Release struct {
    URL                 string          `json:"url"`
    HTMLURL             string          `json:"html_url"`
    AssetsURL           string          `json:"assets_url"`
    UploadURL           string          `json:"upload_url"`
    ID                  int64           `json:"id"`
    TagName             string          `json:"tag_name"`
    TargetCommitish     string          `json:"target_commitish"`
    Name                string          `json:"name"`
    Body                string          `json:"body"`
    Draft               string          `json:"draft"`
    Prerelease          bool            `json:"prerelease"`
    CreatedAt           string          `json:"created_at"`
    PublishedAt         string          `json:"published_at"`
    Assets              []ReleaseAsset  `json:"assets"`
}

type XBMCAddon struct {
    Id          string      `xml:"id,attr"`
    Version     string      `xml:"version,attr"`
    XMLBody     string
    Releases    []Release
}

func (r Release) AssetDownloadURL(filename string) string {
    parts := strings.SplitAfter(r.HTMLURL, "/")
    ghUrl := strings.Join(parts[:len(parts) - 1], "")
    return fmt.Sprintf("%sdownload/%s/%s", ghUrl, r.TagName, filename)
}

var addons map[string]XBMCAddon

func reloadAddons(repos []string) map[string]XBMCAddon {
    tmpAddons := map[string]XBMCAddon{}
    for _, repo := range repos {
        releaseUrl := fmt.Sprintf("https://api.github.com/repos/%s/releases", repo)
        response, err := http.Get(releaseUrl)
        if err != nil {
            // http.Error(w, err.Error(), http.StatusInternalServerError)
            return nil
        }
        body, _ := ioutil.ReadAll(response.Body)
        var releases []Release
        json.Unmarshal(body, &releases)

        var addon XBMCAddon
        lastRelease := releases[0]
        addonUrl := fmt.Sprintf(lastRelease.AssetDownloadURL("addon.xml"))
        response, err = http.Get(addonUrl)
        if err != nil {
            // http.Error(w, err.Error(), http.StatusInternalServerError)
            return nil
        }
        body, _ = ioutil.ReadAll(response.Body)
        xml.Unmarshal(body, &addon)

        scanner := bufio.NewScanner(bytes.NewBuffer(body))
        for scanner.Scan() {
            line := scanner.Text()
            if strings.HasPrefix(line, "<?xml") == true {
                continue
            }
            addon.XMLBody = fmt.Sprintf("%s\n%s", addon.XMLBody, line)
        }
        addon.XMLBody = fmt.Sprintf("%s\n", addon.XMLBody)
        addon.Releases = releases

        tmpAddons[addon.Id] = addon
    }
    return tmpAddons
}

func XBMCRepoMuxer(repos []string) *mux.Router {
    router := mux.NewRouter()
    addons = reloadAddons(repos)

    router.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
        files := []string{}
        for _, addon := range addons {
            for _, asset := range addon.Releases[0].Assets {
                if strings.HasSuffix(asset.Name, ".zip") {
                    files = append(files, asset.Name)
                }
            }
        }
        IndexTemplate.Execute(w, files)
    })

    router.HandleFunc("/addons.xml", func(w http.ResponseWriter, r *http.Request) {
        fmt.Fprintf(w, "<addons>")
        for _, addon := range addons {
            fmt.Fprintf(w, addon.XMLBody)
        }
        fmt.Fprintf(w, "</addons>")
    })

    router.HandleFunc("/addons.xml.md5", func(w http.ResponseWriter, r *http.Request) {
        h := md5.New()
        fmt.Fprintf(h, "<addons>")
        for _, addon := range addons {
            fmt.Fprintf(h, addon.XMLBody)
        }
        fmt.Fprintf(h, "</addons>")
        fmt.Fprintf(w, "%x", h.Sum(nil))
    })

    router.HandleFunc("/{addon_id}/changelog-{version}.txt", func(w http.ResponseWriter, r *http.Request) {
        vars := mux.Vars(r)
        addon := addons[vars["addon_id"]]
        for _, release := range addon.Releases {
            fmt.Fprintf(w, release.Body)
        }
    })

    router.HandleFunc("/{addon_id}/fanart.jpg", func(w http.ResponseWriter, r *http.Request) {
        vars := mux.Vars(r)
        addon := addons[vars["addon_id"]]
        http.Redirect(w, r, addon.Releases[0].AssetDownloadURL("fanart.jpg"), 302)
    })

    router.HandleFunc("/{addon_id}/icon.png", func(w http.ResponseWriter, r *http.Request) {
        vars := mux.Vars(r)
        addon := addons[vars["addon_id"]]
        http.Redirect(w, r, addon.Releases[0].AssetDownloadURL("icon.png"), 302)
    })

    router.HandleFunc("/{addon_id}/{file}.zip", func(w http.ResponseWriter, r *http.Request) {
        vars := mux.Vars(r)
        addon := addons[vars["addon_id"]]
        file := fmt.Sprintf("%s.zip", vars["file"])
        http.Redirect(w, r, addon.Releases[0].AssetDownloadURL(file), 302)
    })

    router.HandleFunc("/reload", func(w http.ResponseWriter, r *http.Request) {
        addons = reloadAddons(repos)
    })

    return router
}

func identifyPlatform(ua string) (string, string) {
    ua = strings.ToLower(ua)
    fmt.Println(ua)

    os := ""
    arch := ""
    if strings.Contains(ua, "mac os x") {
        os = "darwin"
    } else if strings.Contains(ua, "linux") {
        os = "linux"
    } else if strings.Contains(ua, "windows") {
        os = "windows"
    } else if strings.Contains(ua, "android") {
        os = "android"
    }

    if strings.Contains(ua, "armv") {
        arch = "arm"
    } else if strings.Contains(ua, "x86_64") {
        arch = "x64"
    } else if strings.Contains(ua, "x86") || strings.Contains(ua, "wow64") {
        arch = "x86"
    }

    return os, arch
}

func main() {
    http.Handle("/", XBMCRepoMuxer([]string{"steeve/xbmctorrent"}))
    fmt.Println("listening...")
    err := http.ListenAndServe("0.0.0.0:"+os.Getenv("PORT"), nil)
    if err != nil {
        panic(err)
    }
}
