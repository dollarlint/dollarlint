package main

import "testing"

func TestNextPatchReleaseTagSortsSemverNumerically(t *testing.T) {
	tags := []string{"v0.1.9", "v0.1.10", "v0.1.2"}
	if got := compareReleaseTags(tags[1], tags[0]); got <= 0 {
		t.Fatalf("v0.1.10 should sort after v0.1.9, got %d", got)
	}
	next, err := nextPatchReleaseTag([]string{"v0.1.10"})
	if err != nil {
		t.Fatal(err)
	}
	if next != "v0.1.11" {
		t.Fatalf("next tag = %q, want v0.1.11", next)
	}
}

func TestReleaseWingetBranch(t *testing.T) {
	if got := releaseWingetBranch("v0.1.10"); got != "dollarlint-0.1.10" {
		t.Fatalf("branch = %q", got)
	}
}

func TestReleaseWingetPRCheckRequiresReadyFilledBody(t *testing.T) {
	pr := ghWingetPR{
		State:   "OPEN",
		IsDraft: false,
		Body: "Updates `DollarLint.DollarLint` to version `0.1.10`.\n" +
			"Automated Windows validation: https://example.test/run\n" +
			"- [x] Signed the [Contributor License Agreement]\n" +
			"- [x] This PR only modifies one (1) manifest\n" +
			"- [x] Tested manifest with `winget install --manifest <path>`\n",
	}
	check := releaseWingetPRCheck(pr, "v0.1.10")
	if check["ok"] != true {
		t.Fatalf("check = %+v", check)
	}
	pr.IsDraft = true
	check = releaseWingetPRCheck(pr, "v0.1.10")
	if check["ok"] != false {
		t.Fatalf("draft PR should not be ok: %+v", check)
	}
}

func TestAzureBuildRefFromURL(t *testing.T) {
	ref, err := azureBuildRefFromInput("https://dev.azure.com/shine-oss/8b78618a-7973-49d8-9174-4360829d979b/_build/results?buildId=318822")
	if err != nil {
		t.Fatal(err)
	}
	if ref.BuildID != "318822" {
		t.Fatalf("build id = %q", ref.BuildID)
	}
	if ref.APIBase != wingetAzureProjectBaseURL {
		t.Fatalf("api base = %q", ref.APIBase)
	}
}

func TestLatestWingetValidationBuildPrefersWingetbot(t *testing.T) {
	comments := []ghPRComment{
		{
			Body: "Debug build https://dev.azure.com/shine-oss/8b78618a-7973-49d8-9174-4360829d979b/_build/results?buildId=1",
		},
		{
			Body: "Validation Pipeline Run [WinGetSvc-Validation](https://dev.azure.com/shine-oss/8b78618a-7973-49d8-9174-4360829d979b/_build/results?buildId=318822)",
		},
	}
	comments[1].Author.Login = "wingetbot"
	ref, ok := latestWingetValidationBuild(comments)
	if !ok {
		t.Fatal("expected validation build")
	}
	if ref.BuildID != "318822" {
		t.Fatalf("build id = %q", ref.BuildID)
	}
}

func TestReleaseWingetValidationProgressRecognizesInstallerValidation(t *testing.T) {
	timeline := &azureTimeline{Records: []azureTimelineRecord{{Type: "Phase", Name: "Installer Validation", State: "inProgress"}}}
	got := releaseWingetValidationProgress(azureBuild{Status: "inProgress"}, timeline)
	if got != 75 {
		t.Fatalf("progress = %d, want 75", got)
	}
}
