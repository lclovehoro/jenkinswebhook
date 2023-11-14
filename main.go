package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strconv"

	"github.com/bndr/gojenkins"
	"k8s.io/klog/v2"
)

type jJob struct {
	JenkinsUrl   string
	JenkinsUser  string
	JenkinsToken string
	*gojenkins.Jenkins
	context.Context
}

type JobInfo struct {
	Name        string `json:"name"`
	BuildNumber string `json:"buildnumber"`
	WebhookId   string `json:"webhookid"`
	JobType     string `json:"jobtype"`
	Status      string `json:"status"`
	Error       string `json:"error"`
}

var j *jJob
var jobInfo *JobInfo

func main() {
	j = NewjJob()
	http.HandleFunc("/jenkins/webhook", defaultHandler)
	http.ListenAndServe(":8180", nil)
}

func defaultHandler(w http.ResponseWriter, r *http.Request) {
	webhookId := r.URL.Query().Get("webhookId")
	jobType := r.URL.Query().Get("jobType")
	jobName := r.URL.Query().Get("jobName")
	buildNumber := r.URL.Query().Get("buildNumber")
	jobInfo = NewJobInfo(jobName, jobType, webhookId, buildNumber)

	buildNu, err := strconv.ParseInt(buildNumber, 10, 64)
	if err != nil {
		klog.Errorf("buildNumber is not a number, err: %v", err)
		jobInfo.Error = err.Error()
		fmt.Fprint(w, *jobInfo)
		return
	}

	data, err := j.GetBuild(j.Context, jobInfo.Name, buildNu)
	if err != nil {
		jobInfo.Error = err.Error()
		klog.Errorf("getJobStatus failed (job: %s, build number: %s), err: %v", jobInfo.Name, jobInfo.BuildNumber, err)
		fmt.Fprint(w, jobInfo.Error)
		return
	}

	jobInfo.Status = data.GetResult()
	klog.Infof("getJobStatus success (job: %s, build number: %s), status: %s", jobInfo.Name, jobInfo.BuildNumber, jobInfo.Status)

	if len(jobInfo.Status) == 0 {
		jobInfo.PostWebhook()
		fmt.Fprint(w, jobInfo.JobType)
		return
	}

	klog.Info("do not repeat click")
	fmt.Fprint(w, "请勿重复点击")

}

func NewjJob() *jJob {
	jenkinsUrl := GetEnvDefault("JENKINS_URL", "http://localhost:8080")
	jenkinsUser := GetEnvDefault("JENKINS_USER", "lclovehoro")
	jenkinsToken := GetEnvDefault("JENKINS_TOKEN", "1116370325f1c09efe0301aaecfacbc739")

	ctx := context.Background()
	jenkins, err := gojenkins.CreateJenkins(nil, jenkinsUrl, jenkinsUser, jenkinsToken).Init(ctx)
	if err != nil {
		klog.Fatalf("failed to init jenkins: %v", err)
		return nil
	}
	return &jJob{
		JenkinsUrl:   jenkinsUrl,
		JenkinsUser:  jenkinsUser,
		JenkinsToken: jenkinsToken,
		Jenkins:      jenkins,
		Context:      ctx,
	}
}

func NewJobInfo(jobname, jobtype, webhookid, buildnumber string) *JobInfo {
	return &JobInfo{
		Name:        jobname,
		BuildNumber: buildnumber,
		JobType:     jobtype,
		WebhookId:   webhookid,
	}
}

func GetEnvDefault(key, defVal string) string {
	val, Ok := os.LookupEnv(key)
	if !Ok {
		klog.Warningf("failed to get env %s, Using default Val : %s", key, defVal)
		return defVal
	}
	return val
}

func (ji *JobInfo) GenerateWebhookUrl() string {
	return j.JenkinsUrl + "/webhook-step/" + ji.WebhookId
}

func (ji *JobInfo) PostWebhook() {
	url := ji.GenerateWebhookUrl()
	jsonData := map[string]string{
		"type": ji.JobType,
	}
	jsonBody, _ := json.Marshal(jsonData)
	requestBody := bytes.NewBuffer(jsonBody)
	request, _ := http.NewRequest("POST", url, requestBody)
	request.Header.Set("Authorization", GetEnvDefault("WebhookToken", "betawm.com"))

	client := &http.Client{}
	resp, err := client.Do(request)
	if err != nil {
		klog.Errorf("failed to post jenkins job webhook %s", err)
		ji.Error = err.Error()
		return
	}
	defer resp.Body.Close()

	responseData, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		klog.Errorf("failed to read jenkins webhook response body %s", err)
		ji.Error = err.Error()
		return
	}

	klog.Infof("successed to read jenkins webhook response body %v", responseData)
}
