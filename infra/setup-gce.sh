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

echo "=== Reserving static IP ==="
gcloud compute addresses create slack-claude-agent-ip \
  --region=asia-northeast1 \
  --project=$PROJECT_ID

echo "=== Creating firewall rule ==="
gcloud compute firewall-rules create allow-https \
  --allow=tcp:443 \
  --target-tags=allow-https \
  --project=$PROJECT_ID

echo "=== Creating instance ==="
gcloud compute instances create $INSTANCE_NAME \
  --zone=$ZONE \
  --machine-type=e2-small \
  --image-family=ubuntu-2204-lts \
  --image-project=ubuntu-os-cloud \
  --service-account=$SERVICE_ACCOUNT \
  --scopes=cloud-platform \
  --tags=allow-https \
  --address=slack-claude-agent-ip \
  --metadata-from-file=startup-script=startup-script.sh \
  --project=$PROJECT_ID

echo "=== Done ==="
echo "Static IP:"
gcloud compute addresses describe slack-claude-agent-ip \
  --region=asia-northeast1 \
  --format="value(address)" \
  --project=$PROJECT_ID
