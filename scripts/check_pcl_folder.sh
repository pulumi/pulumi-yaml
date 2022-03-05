#!/usr/bin/env bash

LOGS=logs.txt
PROG=$(which pulumi-language-yaml)
if ! [ $? -eq 0 ]; then
    echo "Could not find pulumi-language-yaml."
    exit 1
fi

if [ $# -eq 0 ]; then
    echo "Valid arguments are"
    echo "`convert`"
    echo `analyze`
fi

echo Found pulumi-language-yaml on path at $PROG
echo Log file is $LOGS


convert() {
    for f in $(find . -name '*.pp'); do
        err=$($PROG --convert $f 2>&1 > /dev/null)
        if ! [ $? -eq 0 ]; then
            msg="$f    $err"
            echo "$msg"
            echo "$msg" >> "$LOGS"
        fi
    done
}

analyze() {
    ERR_NO=$(grep '\./.*\.pp' logs.txt | wc -l)
    echo $ERR_NO Errors out of $(find . -name '*.pp' | wc -l) examples
    MISSING_FN=$(grep -o 'YAML does not support Fn::[a-zA-Z]*' logs.txt | awk '{print $5}')
    echo "$(echo $MISSING_FN | wc -w) Failures are due to missing functions"
    for fn in $(echo $MISSING_FN | tr ' ' '\n' | sort -u); do
        echo "$fn needed $(echo $MISSING_FN | tr ' ' '\n' | grep -c $fn)"
    done
}

case "$1" in
    convert)
        convert
        ;;
    analyze)
        analyze
        ;;
esac
