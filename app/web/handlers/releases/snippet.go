package releases

const buildWorkflowUses = "klzn-labs/kalzone-actions/.github/workflows/build.yml@main"

const callerWorkflow = `name: Platform build
run-name: platform release ${{ inputs.release_id }}

on:
  workflow_dispatch:
    inputs:
      release_id:
        required: true
      image:
        required: true
      platform_url:
        required: true

permissions:
  contents: read
  id-token: write

jobs:
  build:
    uses: ` + buildWorkflowUses + `
    with:
      release_id: ${{ inputs.release_id }}
      image: ${{ inputs.image }}
      platform_url: ${{ inputs.platform_url }}
    secrets: inherit
`
