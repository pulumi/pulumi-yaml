#!/usr/bin/env bash

LOGS=logs.txt
PROG=$(which pulumi-language-yaml)
if ! [ $? -eq 0 ]; then
    echo "Could not find pulumi-language-yaml."
    exit 1
fi

read -r -d '' HELP_TEXT <<EOF
     This script helps collect data on programgen in examples.
     There are two valid arguments.
     1. convert - finds all pcl files and converts them into YAML in memory.
        Error messages are output to $LOGS
     2. analyze - performs analysis on $LOGS.
EOF

if [ $# -eq 0 ]; then
    echo "$HELP_TEXT"
    exit 1
fi

echo Found pulumi-language-yaml on path at $PROG
echo Log file is $LOGS


convert() {
    for f in $(find . -name '*.pp'); do
        err=$($PROG --convert $f 2>&1 > /dev/null)
        if ! [ $? -eq 0 ]; then
            echo "$err"
            echo "$err" >> "$LOGS"
        fi
    done
}

analyze() {
    ERR_NO=$(grep -o '\./.*\.pp' logs.txt | sort -u | wc -l)
    echo $ERR_NO Errors out of $(find . -name '*.pp' | wc -l) examples
    MISSING_FN=$(grep 'YAML does not support Fn::[a-zA-Z]*' $LOGS | awk '{print $1 $13}' | sed 's/\.pp:.*:Fn::/.pp!!Fn::/')
    MISSING_FN_NUM=$(echo $MISSING_FN | tr ' ' '\n' | sed 's/!!.*//' | sort -u | wc -l | xargs)
    echo "$MISSING_FN_NUM failures are due to missing functions:"
    for fn in $(echo $MISSING_FN | tr ' ' '\n' | sed 's/.*!!//' | sort -u); do
        echo "    $fn was needed by $(echo $MISSING_FN | tr ' ' '\n' | grep -c $fn) examples."
    done

    UNIMPLEMENTED_EXPR=$(grep 'Unimplimented!' $LOGS | awk '{print $1"!!"$7}')

    echo "$(echo $UNIMPLEMENTED_EXPR | tr ' ' '\n' | sed 's/!!.*//' | sort -u | wc -w | xargs) failures are due to missing expressions:"
    for expr in $(echo $UNIMPLEMENTED_EXPR | tr ' ' '\n' | sed 's/.*!!//' | sort -u); do
        echo "    $expr was needed by $(echo $UNIMPLEMENTED_EXPR | tr ' ' '\n' | grep -c $expr) examples."
    done

    echo "Splat Expression needed for $(grep -c 'Splat;' $LOGS | xargs) examples."
    echo "For Expression needed for $(grep -c 'For;' $LOGS | xargs) examples."
    grep 'panic' $LOGS
}

case "$1" in
    convert)
        convert
        ;;
    analyze)
        analyze
        ;;
    *)
        echo "$HELP_TEXT"
        exit 1
        ;;
esac
