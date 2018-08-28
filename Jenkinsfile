@Library('containership-jenkins@v3')
import io.containership.*

def dockerUtils = new Docker(this)
def gitUtils = new Git(this)
def npmUtils = new Npm(this)
def pipelineUtils = new Pipeline(this)
def kubectlUtils = new Kubectl(this)

pipelineUtils.jenkinsWithNodeTemplate {
    def docker_org = 'containership'
    def docker_name = 'infrastructure-controller'

    def dockerfile_test = 'Dockerfile.test'

    def dockerfile = 'Dockerfile.jenkins'

    def docker_repo = "${docker_org}/${docker_name}"

    def deploy_branch = 'master'

    // will be set in checkout stage
    def git_commit
    def git_branch
    def git_tag
    def is_tag_build = false
    def is_pr_build = false
    def docker_image_tag

    def docker_test_image_id
    def docker_image_id

    stage('Checkout') {
        def scmVars = checkout scm
        env.GIT_COMMIT = scmVars.GIT_COMMIT
        env.GIT_BRANCH = scmVars.GIT_BRANCH
        git_commit = scmVars.GIT_COMMIT
        git_branch = scmVars.GIT_BRANCH
        is_tag_build = gitUtils.isTagBuild(git_branch)
        is_pr_build = gitUtils.isPRBuild(git_branch)

        if(is_tag_build) {
            git_tag = scmVars.GIT_BRANCH
            docker_image_tag = git_tag
        } else {
            docker_image_tag = git_commit
        }
    }

    stage('Test Preparation') {
        container('docker') {
            sh "docker version"

            dockerUtils.buildImage("${docker_repo}:${docker_image_tag}-test", dockerfile_test)
            docker_test_image_id = dockerUtils.getImageId(docker_repo, "${docker_image_tag}-test")
        }
    }

    parallel(
        lint: {
            stage('Test - Linting') {
                container('docker') {
                    dockerUtils.runShellCommand(docker_test_image_id, 'go get -u github.com/golang/lint/golint && PATH=$PATH:/go/bin && make lint')
                }
            }
        },
        test: {
            stage('Test - Testing') {
                container('docker') {
                    dockerUtils.runShellCommand(docker_test_image_id, 'make test')
                }
            }
        },
        vet: {
            stage('Test - Vet') {
                container('docker') {
                    dockerUtils.runShellCommand(docker_test_image_id, 'make vet')
                }
            }
        },
        format: {
            stage('Test - Formatting') {
                container('docker') {
                    dockerUtils.runShellCommand(docker_test_image_id, 'make fmt-check')
                }
            }
        },
        verify: {
            stage('Test - Verifying Generated Code') {
                container('docker') {
                    dockerUtils.runShellCommand(docker_test_image_id, 'make verify')
                }
            }
        }
    )

    if(!is_pr_build && git_branch == deploy_branch) {
        stage('Publish Preparation') {
            container('docker') {
                dockerUtils.buildImage("${docker_repo}:${docker_image_tag}-tmp", dockerfile)
                docker_image_id = dockerUtils.getImageId(docker_repo, "${docker_image_tag}-tmp")

                dir('scratch') {
                    // build and copy files from scratch container
                    def controller_container = "extract-controller"
                    dockerUtils.createContainer(controller_container, "${docker_repo}:${docker_image_tag}-tmp")
                    dockerUtils.copyFromContainer(controller_container, "/scripts/containership_login.sh", "containership-login.sh")
                    dockerUtils.copyFromContainer(controller_container, "/etc/ssl/certs/ca-certificates.crt", "ca-certificates.crt")
                    dockerUtils.copyFromContainer(controller_container, "/app/infrastructure-controller", "infrastructure-controller")
                    dockerUtils.removeContainer(controller_container)

                    // create minimal dockerfile
                    sh 'echo "FROM scratch" >> Dockerfile.scratch'
                    sh 'echo "COPY ./ca-certificates.crt /etc/ssl/certs/ca-certificates.crt" >> Dockerfile.scratch'
                    sh 'echo "COPY ./infrastructure-controller ." >> Dockerfile.scratch'
                    sh 'echo "CMD [\\"./infrastructure-controller\\"]" >> Dockerfile.scratch'

                    dockerUtils.buildImage("${docker_repo}:${docker_image_tag}", "./Dockerfile.scratch")
                    docker_image_id = dockerUtils.getImageId(docker_repo, "${docker_image_tag}")
                }
            }
        }

        stage('Publish - Docker') {
            container('docker') {
                dockerUtils.login(pipelineUtils.getDockerCredentialId())
                dockerUtils.push(docker_repo, docker_image_tag)
            }
        }
    }

    stage('Cleanup') {
        container('docker') {
            dockerUtils.cleanup()
        }
    }
}
