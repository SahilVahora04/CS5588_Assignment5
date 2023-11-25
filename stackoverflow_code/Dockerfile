# Using an official Golang runtime as a base image
FROM golang:1.16-alpine

# Setting the working directory inside the container
WORKDIR /go/src/app

# Copying the local package files to the container's workspace
COPY . .

# Installing any dependencies if needed
# RUN go get -d -v ./...

# Building the Go application
RUN go build -o stackoverflow stackoverflow.go

# Exposing the port the application runs on
EXPOSE 8080

# Running the application
CMD ["./stackoverflow"]
