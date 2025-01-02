# lan
Git hook for executing tests on Git commit of go packages.

This hook executes the tests of the packages in the folder and subfolders of the git repository.
If any test fails then the commit is prevented.

# Install on Windows
Clone this repository and run ```go build```. Take the generated executable and place it in the *.git/hooks* folder of your repository.
Rename the executable to pre-commit.exe.

# Install on Linux
Clone this repository and run ```go build```. Take the generated executable and place it in the *.git\hooks* folder of your repository.
Rename the executable to pre-commit and give it execution permission.