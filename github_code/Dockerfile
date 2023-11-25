# Using an official Golang runtime as a base image
FROM golang:1.16-alpine

# Setting the working directory inside the container
WORKDIR /go/src/app

# Copying the local package files to the container's workspace
COPY . .

# Uncomment the following line if your application has dependencies
# RUN go get -d -v ./...

# Building the Go application
RUN go build -o github github.go

# Expose the port the application runs on
EXPOSE 8080

# Run the application
CMD ["./github"]
