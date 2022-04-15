
;;; To fetch the next template, just run `eval-buffer'.

(setq template-list '("alicloud-typescript/"
                      "auth0-typescript/"
                      "aws-native-typescript/"
                      "aws-typescript/"
                      "azure-classic-typescript/"
                      "azure-typescript/"
                      "civo-typescript/"
                      "digitalocean-typescript/"
                      "equinix-metal-typescript/"
                      "gcp-typescript/"
                      "github-typescript/"
                      "google-native-typescript/"
                      "kubernetes-typescript/"
                      "linode-typescript/"
                      "openstack-typescript/"
                      "typescript/"))

(require 'seq)

(defun find-next-template ()
  "Find the next template to copy over."
  (let ((existing (directory-files "." nil "^[^.]")))
    (seq-reduce (lambda (o s)
                  (or o (unless (or
                                 (seq-contains-p existing s #'string-equal)
                                 (not (string-match-p "yaml" s)))
                          s))
                  )
                (mapcar (lambda (x) (string-replace "typescript" "yaml" x))
                        (directory-files "../../templates" nil "^[^.]"))
                nil)))

(defun start-next-template (src dst)
  "Copy the template from SRC to DST and open DST."
  (copy-directory src dst)
  (find-file-other-window dst))

(let ((next (find-next-template)))
  (if next
      (progn
        (message "Starting on template %s" next)
      (start-next-template (concat "../../templates/" (string-replace "yaml" "typescript" next)) next))
    (message "No templates remaining")))

;;; End
