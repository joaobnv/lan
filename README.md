# lan
Git hook for executing tests on Git commit of go packages.

This hook executes the tests of the packages in the folder and subfolders of the git repository.
If any test fails then the commit is denied. The timeout for the tests is 30s for each package.

This hook checks if the package needs tests but doesn't have them. If this occurs then the commit is denied.

# Install on Windows
Clone this repository and run ```go build``` (requires Go version 1.24rc1 or higher). Take the generated executable
and place it in the *.git\hooks* folder of your repository. Rename the executable to *pre-commit.exe*.

# Install on Linux
Clone this repository and run ```go build``` (requires Go version 1.24rc1 or higher). Take the generated executable and
place it in the *.git/hooks* folder of your repository. Rename the executable to *pre-commit* and give it execution permission.