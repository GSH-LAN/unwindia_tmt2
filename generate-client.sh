#/bin/bash

go install github.com/deepmap/oapi-codegen/v2/cmd/oapi-codegen@latest
go install golang.org/x/tools/cmd/goimports@latest

temp_swagger=/tmp/swagger_tmt2.json
temp_client=/tmp/tmt2_client.go

# check if swagger.json already exists and delete it
[ -f $temp_swagger ] && rm $temp_swagger


curl -o $temp_swagger https://raw.githubusercontent.com/JensForstmann/tmt2/main/backend/swagger.json



# since current (02.02.2024) version of oapi-codegen does not fully support anyof stuff with integers we grep the error output and modify the generated code
oapi-codegen -generate types,client -package tmt2 $temp_swagger > src/tmt2/client.gen.go 2> $temp_client

# remove first line from generated code
awk 'FNR > 1' $temp_client > $temp_client.tmp

# remove last 2 lines from generated code
awk -v n=2 'NR==FNR{total=NR;next} FNR==total-n+1{exit} 1' $temp_client.tmp $temp_client.tmp > $temp_client
sed -i -e 's/200_TmtPort/int/g' $temp_client

mv $temp_client pkg/tmt2-go/client.gen.go

goimports -w pkg/tmt2-go/client.gen.go
