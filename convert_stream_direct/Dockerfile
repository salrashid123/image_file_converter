
FROM node:10 AS build-env
RUN apt-get update -y && apt-get install -y libmagickwand-dev
ADD . /app
WORKDIR /app
RUN npm install request express  gm @google-cloud/storage

WORKDIR /app
EXPOSE 8080
CMD ["index.js"]