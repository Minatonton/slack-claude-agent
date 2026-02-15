#!/bin/bash
set -e

PROJECT_ID="your-project-id"
ZONE="asia-northeast1-b"
INSTANCE_NAME="slack-claude-agent"
SERVICE_ACCOUNT="slack-claude-agent@${PROJECT_ID}.iam.gserviceaccount.com"

echo "=== Creating service account ==="
gcloud iam service-accounts create slack-claude-agent \
  --display-name="Slack Claude Agent" \
  --project=$PROJECT_ID

echo "=== Granting IAM roles ==="
gcloud projects add-iam-policy-binding $PROJECT_ID \
  --member="serviceAccount:${SERVICE_ACCOUNT}" \
  --role="roles/aiplatform.user"

gcloud projects add-iam-policy-binding $PROJECT_ID \
  --member="serviceAccount:${SERVICE_ACCOUNT}" \
  --role="roles/secretmanager.secretAccessor"

echo "=== Creating instance ==="
gcloud compute instances create $INSTANCE_NAME \
  --zone=$ZONE \
  --machine-type=e2-small \
  --image-family=ubuntu-2204-lts \
  --image-project=ubuntu-os-cloud \
  --service-account=$SERVICE_ACCOUNT \
  --scopes=cloud-platform \
  --metadata-from-file=startup-script=startup-script.sh \
  --project=$PROJECT_ID

echo "=== Done ==="
echo "Instance created. No static IP or firewall rules needed for Socket Mode."
