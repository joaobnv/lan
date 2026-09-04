# Lan
Git hook for executing tests on commit of go packages.

Lan runs the tests of the packages in the git repository.
If any test fails then Lan denies the commit. The timeout for the tests is 240s for each package.
Lan also verifies if the coverage of the tests is 100%. If not then Lan denies the commit.

Lan runs [staticcheck](https://staticcheck.dev/) in each package, if it is installed. If it exits with a
non-zero value then Lan denies the commit.

Lan runs `go vet` in each package. If it exits with a non-zero value then Lan denies the commit.

Lan checks if some package needs tests but doesn't have them. If this occurs then Lan denies the commit.

# Install on Windows
Run (Cmd) `set GOEXPERIMENT=jsonv2` and `go install github.com/joaobnv/lan@latest`. Then copy the installed command and place it
in the *.git\hooks* folder of your repository. Then rename it to *pre-commit.exe*.

# Install on Linux
Run (Bash) `GOEXPERIMENT=jsonv2 go install github.com/joaobnv/lan@latest`. Then copy the installed command and place it
in the *.git/hooks* folder of your repository. Then rename it to *pre-commit* and give it execution permission.

# Caveats
Lan doesn't support folders with multiple non-test packages. That is, if the folder has packages `foo` and `bar`
then Lan will not work. But if the folder has packages `foo` and `foo_test` then lan will work.

# Miscellaneous
Do you need to run Lan but don't have anything to commit? Do this: install Lan, with `go install`, but don't copy it to
the hooks folder. Then go to the folder of your repository and invoke Lan with the `lan` command.

Don't want to have to copy Lan to your hooks folder every time Lan is updated? Challenge: create a way so you only need
to run the `go install` command and all your repositories will have the new version of Lan without you having to copy it.

# Change log

## 0.1.0

Now Lan runs the staticcheck.