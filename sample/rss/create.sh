#!/bin/bash

ko apply -f pkg/sources/rss/

kubectl apply -f sample/rss/flow.yaml
