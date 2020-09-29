## Using Cloud Run to Stream proxy and resize images from GCS 

`...and now with CDN`


Sample app that sets up a way to dynamically proxy and manipullate/resize images stored on GCS.


`client -> Cloud CDN -> L7LB -> Cloud Run -> GCS`


This sample uses nodejs to stream process the images which means the images are not loaded into Cloud Run's memory.

The following step will set all this up


### Setup

```bash
export PROJECT_ID=`gcloud config get-value core/project`
export PROJECT_NUMBER=`gcloud projects describe $PROJECT_ID --format="value(projectNumber)"`
export SA_EMAIL=sa-run@$PROJECT_ID.iam.gserviceaccount.com
export REGION=us-central1

gcloud compute addresses create img-ip --global  
export ADDRESS=`gcloud compute addresses describe img-ip --global --format="value(address)"`

gcloud iam service-accounts create sa-run --display-name "Cloud Run Image Service Account" --project $PROJECT_ID
gsutil mb gs://$PROJECT_ID-img
gsutil iam ch serviceAccount:$SA_EMAIL:objectViewer gs://$PROJECT_ID-img

gsutil -h "Content-Type:image/png" -h "Cache-Control:public, max-age=7200" cp test.png gs://$PROJECT_ID-img/

gcloud builds submit --tag gcr.io/$PROJECT_ID/imgproxy .
```


```bash
gcloud beta run deploy imgproxy \
    --image gcr.io/$PROJECT_ID/imgproxy --set-env-vars GCS_BUCKET="$PROJECT_ID-img" \
    --allow-unauthenticated  --region $REGION  --platform=managed \
    --service-account $SA_EMAIL

gcloud beta compute network-endpoint-groups create imgproxy-neg     --region=$REGION  \
     --network-endpoint-type=SERVERLESS    \
     --cloud-run-service=imgproxy

gcloud compute backend-services create imgproxy-service     --global

gcloud beta compute backend-services add-backend imgproxy-service   \
    --global     --network-endpoint-group=imgproxy-neg  \
    --network-endpoint-group-region=$REGION

gcloud compute backend-services update imgproxy-service --global --enable-cdn

gcloud compute ssl-certificates create img-cert --certificate images.crt --private-key images.key

gcloud compute url-maps create img-map --default-service imgproxy-service  

gcloud compute url-maps add-path-matcher img-map  --path-matcher-name=img --default-service imgproxy-service
gcloud compute url-maps add-host-rule  img-map --hosts=images.domain.com --path-matcher-name=img  --global 

gcloud compute target-https-proxies create img-lb-proxy --url-map=img-map --ssl-certificates=img-cert --global  
gcloud compute forwarding-rules create img-content-rule --address  $ADDRESS --global --target-https-proxy img-lb-proxy --ports 443  
```

### Test

Direct:

```bash
curl -v --cacert tls-ca.crt -H "Host: images.domain.com" \
  --resolve images.domain.com:443:$ADDRESS https://images.domain.com/images/test.png

< HTTP/2 200 
< x-powered-by: Express
< content-type: image/png
< cache-control: public, max-age=7200
< date: Tue, 29 Sep 2020 20:41:28 GMT
< server: Google Frontend
< via: 1.1 google
< alt-svc: clear

< HTTP/2 200 
< x-powered-by: Express
< content-type: image/png
< date: Tue, 29 Sep 2020 20:41:28 GMT
< server: Google Frontend
< via: 1.1 google
< cache-control: public, max-age=7200
< content-length: 33344
< age: 23                           <<<<<<<<<<<<<<<
< alt-svc: clear

```

Resize

```bash
curl -v --cacert tls-ca.crt -H "Host: images.domain.com" \
   --resolve images.domain.com:443:$ADDRESS https://images.domain.com/images/test.png?w=200&l=200

< HTTP/2 200 
< x-powered-by: Express
< content-type: image/png
< cache-control: public, max-age=7200
< date: Tue, 29 Sep 2020 20:42:37 GMT
< server: Google Frontend
< via: 1.1 google
< alt-svc: clear
< 

< HTTP/2 200 
< x-powered-by: Express
< content-type: image/png
< date: Tue, 29 Sep 2020 20:42:37 GMT
< server: Google Frontend
< via: 1.1 google
< cache-control: public, max-age=7200
< content-length: 6243
< age: 53                            <<<<<<<<<<<<<<<<
< alt-svc: clear
< 
```
